// This file reads HTTP responses and limits compressed and decoded data.

package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxBody = 64 << 20

// httpReader bounds both wire and decoded bytes for each HTTP response.
type httpReader struct {
	client *http.Client
	limit  int64
}

func newHTTPReader() *httpReader {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	return &httpReader{
		client: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("HTTP redirects are not supported")
			},
		},
	}
}

func (r *httpReader) get(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("network request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from GCS", response.StatusCode)
	}
	limit := r.limit
	if limit <= 0 {
		limit = maxBody
	}
	raw, err := readBounded(response.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("read raw response: %w", err)
	}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		compressed, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("gzip header: %w", err)
		}
		defer compressed.Close()
		body, err := readBounded(compressed, limit)
		if err != nil {
			return nil, fmt.Errorf("read/decompress response: %w", err)
		}
		return body, nil
	}
	return raw, nil
}

// readBounded rejects input that exceeds the byte limit.
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}
