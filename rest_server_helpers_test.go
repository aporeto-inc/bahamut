// Copyright 2019 Aporeto Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bahamut

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.aporeto.io/elemental"
)

func TestRestServerHelpers_commonHeaders(t *testing.T) {

	Convey("Given I create a http.ResponseWriter", t, func() {

		w := httptest.NewRecorder()

		Convey("When I use setCommonHeader using json", func() {

			setCommonHeader(w, elemental.EncodingTypeJSON)

			Convey("Then the common headers should be set", func() {
				So(w.Header().Get("Accept"), ShouldEqual, "application/msgpack,application/json")
				So(w.Header().Get("Content-Type"), ShouldEqual, "application/json; charset=UTF-8")
			})
		})

		Convey("When I use setCommonHeader using msgpack", func() {

			setCommonHeader(w, elemental.EncodingTypeMSGPACK)

			Convey("Then the common headers should be set", func() {
				So(w.Header().Get("Accept"), ShouldEqual, "application/msgpack,application/json")
				So(w.Header().Get("Content-Type"), ShouldEqual, "application/msgpack")
			})
		})
	})
}

func TestRestServerHelper_notFoundHandler(t *testing.T) {

	Convey("Given I call the notFoundHandler", t, func() {

		h := http.Header{}
		h.Add("Origin", "toto")

		w := httptest.NewRecorder()
		makeNotFoundHandler()(w, &http.Request{Header: h, URL: &url.URL{Path: "/path"}})

		Convey("Then the response should be correct", func() {
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

func TestRestServerHelper_writeHTTPResponse(t *testing.T) {

	Convey("Given I have a response with a nil response", t, func() {

		w := httptest.NewRecorder()

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, nil)

			Convey("Then the code should be 0", func() {
				So(code, ShouldEqual, 0)
			})
		})
	})

	Convey("Given I have a response with a redirect", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())
		r.Redirect = "https://la.bas"

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, r)

			Convey("Then the should header Location should be set", func() {
				So(w.Header().Get("location"), ShouldEqual, "https://la.bas")
			})

			Convey("Then the code should be 302", func() {
				So(code, ShouldEqual, 302)
			})
		})
	})

	Convey("Given I have a response with no data", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())

		r.StatusCode = http.StatusNoContent

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, r)

			Convey("Then the should headers should be correct", func() {
				So(w.Header().Get("X-Count-Total"), ShouldEqual, "0")
				So(w.Header().Get("X-Messages"), ShouldEqual, "")
			})

			Convey("Then the code should correct", func() {
				So(w.Code, ShouldEqual, http.StatusNoContent)
			})

			Convey("Then the code should be http.StatusNoContent", func() {
				So(code, ShouldEqual, http.StatusNoContent)
			})
		})
	})

	Convey("Given I have a response messages", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())

		r.Messages = []string{"msg1", "msg2"}
		r.StatusCode = 200

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, r)

			Convey("Then the should header message should be set", func() {
				So(w.Header().Get("X-Messages"), ShouldEqual, "msg1;msg2")
			})

			Convey("Then the code should be http.StatusNoContent", func() {
				So(code, ShouldEqual, http.StatusOK)
			})
		})
	})

	Convey("Given I have a response next", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())

		r.Next = "next"
		r.StatusCode = 200

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, r)

			Convey("Then the should header message should be set", func() {
				So(w.Header().Get("X-Next"), ShouldEqual, "next")
			})

			Convey("Then the code should be http.StatusNoContent", func() {
				So(code, ShouldEqual, http.StatusOK)
			})
		})
	})

	Convey("Given I have a response with data", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())

		r.StatusCode = http.StatusOK
		r.Data = []byte("hello")

		Convey("When I call writeHTTPResponse", func() {

			code := writeHTTPResponse(w, r)

			Convey("Then the body should be correct", func() {
				So(w.Header().Get("X-Count-Total"), ShouldEqual, "0")
				So(w.Header().Get("X-Messages"), ShouldEqual, "")
				So(w.Body.String(), ShouldEqual, string(r.Data))
			})

			Convey("Then the code should be http.StatusNoContent", func() {
				So(code, ShouldEqual, http.StatusOK)
			})
		})
	})

	Convey("Given I have a some cookies", t, func() {

		w := httptest.NewRecorder()
		r := elemental.NewResponse(elemental.NewRequest())
		r.Cookies = []*http.Cookie{
			{
				Name:  "ca",
				Value: "ca",
			},
			{
				Name:  "cb",
				Value: "cb",
			},
		}

		Convey("When I call writeHTTPResponse", func() {

			writeHTTPResponse(w, r)

			Convey("Then the should header message should be set", func() {
				So(w.Header()["Set-Cookie"], ShouldResemble, []string{"ca=ca", "cb=cb"})
			})
		})
	})
}

func Test_extractAPIVersion(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name        string
		args        args
		wantVersion int
		wantErr     bool
	}{
		{
			"no path",
			args{
				"",
			},
			0,
			false,
		},
		{
			"valid unversionned with no heading /",
			args{
				"objects",
			},
			0,
			false,
		},
		{
			"valid unversionned with heading /",
			args{
				"/objects",
			},
			0,
			false,
		},
		{
			"valid versionned with no heading /",
			args{
				"v/4/objects",
			},
			4,
			false,
		},
		{
			"valid versionned with heading /",
			args{
				"/v/4/objects",
			},
			4,
			false,
		},
		{
			"invalid versionned with no heading /",
			args{
				"v/dog/objects",
			},
			0,
			true,
		},
		{
			"invalid versionned with heading /",
			args{
				"/v/dog/objects",
			},
			0,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, err := extractAPIVersion(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractAPIVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("extractAPIVersion() = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}
