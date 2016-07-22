// Author: Antoine Mercadal
// See LICENSE file for full LICENSE
// Copyright 2016 Aporeto.

package bahamut

import "net/http"

// An APIServerConfig represents the configuration for the APIServer.
type APIServerConfig struct {

	// EnableProfiling defines if the profiling server should
	// be created. You should only use this when debugging.
	EnableProfiling bool

	// HealthEndpoint represents the api endpoint to use for the
	// health check handler.
	HealthEndpoint string

	// HealthHandler is the http handler to call when a user accesses
	// the HealthEndpoint.
	HealthHandler http.HandlerFunc

	// HealthListenAddress is the custom listening address to use to
	// access the HealthHandler. This is only used if Bahamut is started
	// without TLS.
	HealthListenAddress string

	// ListenAddress is the general listening address for the API server as
	// well as the PushServer.
	ListenAddress string

	// Routes holds all the routes Bahamut will serve. Those routes
	// Are normally autogenerated from specifications.
	Routes []*Route

	// TLSCAPath is the path the CA certificates used in various place
	// of Bahamut.
	TLSCAPath string

	// TLSCertificatePath is the path of the certificate used to establish
	// a TLS connection.
	//
	// This is optional. If you don't provide it, then Bahamut will start
	// without TLS support.
	TLSCertificatePath string

	// TLSKeyPath is the path of the private key used to establish
	// a TLS connection.
	//
	// This is optional. If you don't provide it, then Bahamut will start
	// without TLS support.
	TLSKeyPath string

	// Disabled defines if the API system should be enabled.
	Disabled bool
}

// A PushServerConfig contains the configuration for the Bahamut Push Server.
type PushServerConfig struct {

	// Service defines the pubsub service to user.
	Service PubSubServer

	// Topic defines the default notification topic to use.
	Topic string

	// SessionsHandler defines the handler that will be used to
	// manage push session lifecycle.
	SessionsHandler PushSessionsHandler

	// Disabled defines if the Push system should be enabled.
	Disabled bool
}
