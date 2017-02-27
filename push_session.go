// Author: Antoine Mercadal
// See LICENSE file for full LICENSE
// Copyright 2016 Aporeto.

package bahamut

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/aporeto-inc/elemental"
	"github.com/satori/go.uuid"
	"golang.org/x/net/websocket"
)

type pushSessionType int

const (
	pushSessionTypeEvent pushSessionType = iota + 1
	pushSessionTypeAPI
)

// PushSession represents a client session.
type PushSession struct {
	Identity   []string
	Parameters url.Values
	Headers    http.Header

	config            Config
	events            chan *elemental.Event
	id                string
	processorFinder   processorFinder
	pushEventsFunc    func(...*elemental.Event)
	requests          chan *elemental.Request
	filters           chan *elemental.PushFilter
	socket            *websocket.Conn
	startTime         time.Time
	stopAll           chan bool
	stopRead          chan bool
	stopWrite         chan bool
	sType             pushSessionType
	unregisterFunc    func(*PushSession)
	filter            *elemental.PushFilter
	currentFilterLock *sync.Mutex
}

func newPushSession(ws *websocket.Conn, config Config, unregisterFunc func(*PushSession)) *PushSession {

	return newSession(ws, pushSessionTypeEvent, config, unregisterFunc, nil, nil)
}

func newAPISession(ws *websocket.Conn, config Config, unregisterFunc func(*PushSession), processorFinder processorFinder, pushEventsFunc func(...*elemental.Event)) *PushSession {

	return newSession(ws, pushSessionTypeAPI, config, unregisterFunc, processorFinder, pushEventsFunc)
}

func newSession(ws *websocket.Conn, sType pushSessionType, config Config, unregisterFunc func(*PushSession), processorFinder processorFinder, pushEventsFunc func(...*elemental.Event)) *PushSession {

	var parameters url.Values
	var headers http.Header

	if request := ws.Request(); request != nil {
		parameters = request.URL.Query()
	}

	if config := ws.Config(); config != nil {
		headers = config.Header
	}

	return &PushSession{
		config:            config,
		Identity:          []string{},
		events:            make(chan *elemental.Event),
		Headers:           headers,
		id:                uuid.NewV4().String(),
		Parameters:        parameters,
		processorFinder:   processorFinder,
		pushEventsFunc:    pushEventsFunc,
		requests:          make(chan *elemental.Request, 8),
		filters:           make(chan *elemental.PushFilter, 8),
		currentFilterLock: &sync.Mutex{},
		socket:            ws,
		startTime:         time.Now(),
		stopAll:           make(chan bool, 2),
		stopRead:          make(chan bool, 2),
		stopWrite:         make(chan bool, 2),
		sType:             sType,
		unregisterFunc:    unregisterFunc,
	}
}

// Identifier returns the identifier of the push session.
func (s *PushSession) Identifier() string {

	return s.id
}

// DirectPush will send given events to the session without any further control
// but ensuring the events did not happen before the session has been initialized.
// the ShouldPush method of the eventual bahamut.PushHandler will *not* be called.
//
// For performance reason, this method will *not* check that it is an session of type
// Event. If you direct push to an API session, you will fill up the internal channels until
// it blocks.
//
// This method should be used only if you know what you are doing, and you should not need it
// in the vast majority of all cases.
func (s *PushSession) DirectPush(events ...*elemental.Event) {

	for _, event := range events {

		if event.Timestamp.Before(s.startTime) {
			continue
		}

		s.events <- event
	}

}

func (s *PushSession) readRequests() {

	for {
		var request *elemental.Request

		if err := websocket.JSON.Receive(s.socket, &request); err != nil {
			s.stopAll <- true
			return
		}

		select {
		case s.requests <- request:
		case <-s.stopRead:
			return
		}
	}
}

func (s *PushSession) readFilters() {

	for {
		var filter *elemental.PushFilter

		if err := websocket.JSON.Receive(s.socket, &filter); err != nil {
			s.stopAll <- true
			return
		}

		select {
		case s.filters <- filter:
		case <-s.stopRead:
			return
		}
	}
}

func (s *PushSession) write() {

	for {
		select {
		case event := <-s.events:

			f := s.currentFilter()
			if f != nil && f.IsFilteredOut(event.Identity, event.Type) {
				break
			}

			if err := websocket.JSON.Send(s.socket, event); err != nil {
				s.stopAll <- true
				return
			}

		case <-s.stopWrite:
			return
		}
	}
}

func (s *PushSession) close() {

	s.stopAll <- true
}

func (s *PushSession) listen() {

	switch s.sType {
	case pushSessionTypeAPI:
		s.listenToAPIRequest()
	case pushSessionTypeEvent:
		s.listenToPushEvents()
	default:
		panic("Unknown push session type")
	}
}

func (s *PushSession) currentFilter() *elemental.PushFilter {

	s.currentFilterLock.Lock()
	defer s.currentFilterLock.Unlock()

	if s.filter == nil {
		return nil
	}

	return s.filter.Duplicate()
}

func (s *PushSession) setCurrentFilter(f *elemental.PushFilter) {

	s.currentFilterLock.Lock()
	s.filter = f
	s.currentFilterLock.Unlock()
}

func (s *PushSession) listenToPushEvents() {

	go s.readFilters()
	go s.write()

	defer func() {
		s.stopRead <- true
		s.stopWrite <- true

		s.unregisterFunc(s)
		s.socket.Close()
		s.processorFinder = nil
		s.pushEventsFunc = nil
		s.unregisterFunc = nil
	}()

	for {
		select {
		case filter := <-s.filters:
			s.setCurrentFilter(filter)

		case <-s.stopAll:
			return
		}
	}
}

func (s *PushSession) listenToAPIRequest() {

	go s.write()
	go s.readRequests()

	defer func() {
		s.stopRead <- true
		s.stopWrite <- true

		s.unregisterFunc(s)
		s.socket.Close()
		s.processorFinder = nil
		s.pushEventsFunc = nil
		s.unregisterFunc = nil
	}()

	for {
		select {
		case request := <-s.requests:

			// We backport the token of the session into the request.
			if t := s.Parameters.Get("token"); t != "" {
				request.Username = "Bearer"
				request.Password = t
			}

			switch request.Operation {

			case elemental.OperationRetrieveMany:
				go s.handleRetrieveMany(request)

			case elemental.OperationRetrieve:
				go s.handleRetrieve(request)

			case elemental.OperationCreate:
				go s.handleCreate(request)

			case elemental.OperationUpdate:
				go s.handleUpdate(request)

			case elemental.OperationDelete:
				go s.handleDelete(request)

			case elemental.OperationInfo:
				go s.handleInfo(request)

			case elemental.OperationPatch:
				go s.handlePatch(request)
			}

		case <-s.stopAll:
			return
		}
	}
}

func (s *PushSession) handleEventualPanic(response *elemental.Response) {

	if r := recover(); r != nil {
		writeWebSocketError(
			s.socket,
			response,
			elemental.NewError(
				"Internal Server Error",
				fmt.Sprintf("%v", r),
				"bahamut",
				http.StatusInternalServerError,
			),
		)
	}
}

func (s *PushSession) handleRetrieveMany(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchRetrieveManyOperation(
		request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handleRetrieve(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchRetrieveOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handleCreate(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchCreateOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.pushEventsFunc,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handleUpdate(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchUpdateOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.pushEventsFunc,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handleDelete(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchDeleteOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.pushEventsFunc,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handleInfo(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchInfoOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) handlePatch(request *elemental.Request) {

	response := elemental.NewResponse()
	response.Request = request

	defer s.handleEventualPanic(response)

	ctx, err := dispatchPatchOperation(
		response.Request,
		s.processorFinder,
		s.config.Model.IdentifiablesFactory,
		s.config.Security.RequestAuthenticator,
		s.config.Security.Authorizer,
		s.pushEventsFunc,
		s.config.Security.Auditer,
	)

	if err != nil {
		writeWebSocketError(s.socket, response, err)
		return
	}

	writeWebsocketResponse(s.socket, response, ctx)
}

func (s *PushSession) String() string {

	return fmt.Sprintf("<session id:%s headers: %v parameters: %v>",
		s.id,
		s.Headers,
		s.Parameters,
	)
}
