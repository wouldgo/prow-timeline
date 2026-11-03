package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPReaderCompression(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"plain", "gzip header", "gzip magic", "broken gzip"} {
		t.Run(encoding, func(t *testing.T) {
			t.Parallel()
			reader := testHTTPReader(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if encoding == "plain" {
					_, _ = io.WriteString(response, "example response")
					return
				}
				if encoding == "broken gzip" {
					_, _ = response.Write([]byte{0x1f, 0x8b, 0})
					return
				}
				if encoding == "gzip header" {
					response.Header().Set("Content-Encoding", "gzip")
				}
				compressed := gzip.NewWriter(response)
				_, _ = io.WriteString(compressed, "example response")
				_ = compressed.Close()
			}))
			body, err := reader.get(context.Background(), "https://storage.example/object")
			if encoding == "broken gzip" {
				if err == nil {
					t.Fatal("expected gzip error")
				}
				return
			}
			if err != nil || string(body) != "example response" {
				t.Fatalf("body=%q err=%v", body, err)
			}
		})
	}
}

func TestHTTPReaderResponseBounds(t *testing.T) {
	t.Parallel()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := io.WriteString(writer, strings.Repeat("a", 2048)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name           string
		body           []byte
		encoding, want string
	}{
		{"raw bound", bytes.Repeat([]byte("a"), 513), "", "raw response: response exceeds 512"},
		{"raw gzip bound", bytes.Repeat(compressed.Bytes(), 20), "gzip", "raw response: response exceeds 512"},
		{"decoded bound", compressed.Bytes(), "gzip", "decompress response: response exceeds 512"},
		{"truncated gzip", compressed.Bytes()[:compressed.Len()-5], "gzip", "unexpected EOF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			reader := testHTTPReader(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Accept-Encoding") != "" {
					t.Error("automatic gzip negotiation should be disabled")
				}
				if test.encoding != "" {
					response.Header().Set("Content-Encoding", test.encoding)
				}
				_, _ = response.Write(test.body)
			}))
			reader.limit = 512
			if test.name == "truncated gzip" {
				reader.limit = 4096
			}
			_, err := reader.get(context.Background(), "https://storage.example/object")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want=%s", err, test.want)
			}
		})
	}
}

func TestHTTPReaderRejectsRedirects(t *testing.T) {
	t.Parallel()
	reader := testHTTPReader(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/again", http.StatusFound)
	}))
	if _, err := reader.get(context.Background(), "https://storage.example/object"); err == nil || !strings.Contains(err.Error(), "redirects are not supported") {
		t.Fatalf("redirect error=%v", err)
	}
}

func TestHTTPReaderNetworkFailure(t *testing.T) {
	t.Parallel()
	reader := newHTTPReader()
	sentinel := errors.New("network unavailable")
	reader.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, sentinel })
	if _, err := reader.get(context.Background(), "https://storage.example/object"); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "network request failed") {
		t.Fatalf("network error=%v", err)
	}
}

func TestHTTPReaderCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newHTTPReader()
	if _, err := r.get(ctx, "http://127.0.0.1:1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
}

func TestReadBounded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body  string
		valid bool
	}{{"123", true}, {"1234", true}, {"12345", false}} {
		body, err := readBounded(strings.NewReader(tc.body), 4)
		if (err == nil) != tc.valid || (tc.valid && string(body) != tc.body) {
			t.Fatalf("readBounded(%q) = %q, %v", tc.body, body, err)
		}
	}
}

func TestHTTPReaderDefaults(t *testing.T) {
	t.Parallel()
	r := newHTTPReader()
	transport, ok := r.client.Transport.(*http.Transport)
	if !ok || !transport.DisableCompression {
		t.Fatal("automatic compression must be disabled to bound raw bytes")
	}
	if transport == http.DefaultTransport || r.client.Timeout != 60*time.Second {
		t.Fatal("expected a private transport and 60-second request timeout")
	}
}

func TestHTTPReaderRejectsHTTPErrorWithoutLeakingBody(t *testing.T) {
	t.Parallel()
	r := testHTTPReader(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "internal diagnostic")
	}))
	_, err := r.get(context.Background(), "https://storage.example/object")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403 from GCS") || strings.Contains(err.Error(), "internal diagnostic") {
		t.Fatalf("error = %v", err)
	}
}
