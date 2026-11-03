package fetch

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFetcherIntegration(t *testing.T) {
	t.Parallel()
	job, err := ParseJobURL(sampleURL)
	if err != nil {
		t.Fatal(err)
	}
	first := job.Prefix + "step-one/junit/e2e-timelines_everything_20261102-170355.json"
	second := job.Prefix + "step-two/junit/e2e-timelines_everything_20261102-170355.json.gz"
	reader := testHTTPReader(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/storage/v1/") {
			if request.URL.Query().Get("prefix") != job.Prefix {
				t.Error("wrong listing prefix")
			}
			var listing any
			if request.URL.Query().Get("pageToken") == "" {
				listing = map[string]any{"items": []map[string]string{{"name": first}, {"name": strings.Replace(first, "everything_", "everything-else_", 1)}, {"name": first + ".html"}}, "nextPageToken": "next/+ token"}
			} else {
				if request.URL.Query().Get("pageToken") != "next/+ token" {
					t.Error("page token lost")
				}
				listing = map[string]any{"items": []map[string]string{{"name": second}, {"name": first}, {"name": "outside/e2e-timelines_everything_20261102-170355.json"}}}
			}
			if err := json.NewEncoder(response).Encode(listing); err != nil {
				t.Error(err)
			}
			return
		}
		if !strings.Contains(request.URL.Path, first) && !strings.Contains(request.URL.Path, second) {
			t.Errorf("unexpected download %s", request.URL.Path)
		}
		compressed := gzip.NewWriter(response)
		if _, err := io.WriteString(compressed, testJSON); err != nil {
			t.Error(err)
		}
		if err := compressed.Close(); err != nil {
			t.Error(err)
		}
	}))
	fetch := &Fetcher{loader: NewLoader(&gcsStore{reader: reader, base: "https://storage.example"})}
	data, err := fetch.Load(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Runs) != 2 || data.TestCount != 2 || data.EventCount != 2 {
		t.Fatalf("unexpected dataset %+v", data)
	}
}
