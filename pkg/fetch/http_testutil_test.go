package fetch

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// roundTripFunc replaces only network I/O; http.Client still handles redirects.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func handlerTransport(handler http.Handler) http.RoundTripper {
	return roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})
}

func testHTTPReader(t *testing.T, handler http.Handler) *httpReader {
	t.Helper()
	reader := newHTTPReader()
	reader.client.Transport = handlerTransport(handler)
	return reader
}
