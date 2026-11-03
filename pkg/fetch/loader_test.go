package fetch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/wouldgo/prow-timeline/pkg/timeline"
)

const testJSON = `{"items":[{"source":"E2ETest","locator":{"type":"E2ETest","keys":{"e2e-test":"example"}},"message":{"annotations":{"status":"Passed"}},"from":"2026-11-02T17:11:09Z","to":"2026-11-02T17:11:13Z"}]}`

type stubStore struct {
	names       []string
	bodies      map[string][]byte
	discoverErr error
	downloadErr error
	downloads   []string
	job         JobReference
}

func (s *stubStore) Discover(_ context.Context, job JobReference) ([]string, error) {
	s.job = job
	return s.names, s.discoverErr
}

func (s *stubStore) Download(_ context.Context, job JobReference, name string) ([]byte, error) {
	s.job = job
	s.downloads = append(s.downloads, name)
	return s.bodies[name], s.downloadErr
}

func TestLoaderAssemblesDatasetInDiscoveryOrder(t *testing.T) {
	t.Parallel()
	job := JobReference{URL: sampleURL, Prefix: "artifacts/"}
	nonTest := strings.Replace(testJSON, "E2ETest", "PodState", 1)
	store := &stubStore{
		names: []string{"artifacts/z", "artifacts/a"},
		bodies: map[string][]byte{
			"artifacts/z": []byte(testJSON),
			"artifacts/a": []byte(nonTest),
		},
	}
	data, err := NewLoader(store).Load(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if data.JobURL != job.URL || data.TestCount != 1 || data.EventCount != 2 ||
		len(data.Runs) != 2 || data.Runs[0].Name != "z" || data.Runs[1].Name != "a" ||
		!reflect.DeepEqual(store.downloads, store.names) || store.job != job {
		t.Fatalf("dataset = %+v, downloads = %v, job = %+v", data, store.downloads, store.job)
	}
}

func TestLoaderRejectsIncompleteDatasets(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("store failure")
	for _, tc := range []struct {
		name, second, want       string
		discoverErr, downloadErr error
		limit                    int
	}{
		{name: "discovery", discoverErr: sentinel, want: "store failure"},
		{name: "download", downloadErr: sentinel, want: "download artifacts/first"},
		{name: "invalid second artifact", second: "{}", want: "artifact artifacts/second: missing or empty"},
		{name: "combined byte limit", second: testJSON, limit: 2*len(testJSON) - 1, want: "combined artifacts exceed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &stubStore{
				names:       []string{"artifacts/first", "artifacts/second"},
				bodies:      map[string][]byte{"artifacts/first": []byte(testJSON), "artifacts/second": []byte(tc.second)},
				discoverErr: tc.discoverErr, downloadErr: tc.downloadErr,
			}
			loader := NewLoader(store)
			loader.totalLimit = tc.limit
			data, err := loader.Load(context.Background(), JobReference{Prefix: "artifacts/"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if !reflect.DeepEqual(data, timeline.Dataset{}) {
				t.Fatalf("partial result returned: %+v", data)
			}
			if (tc.discoverErr != nil || tc.downloadErr != nil) && !errors.Is(err, sentinel) {
				t.Fatalf("store error was not preserved: %v", err)
			}
		})
	}
}

func TestLoaderRequiresTests(t *testing.T) {
	t.Parallel()
	store := &stubStore{names: []string{"run"}, bodies: map[string][]byte{
		"run": []byte(strings.Replace(testJSON, "E2ETest", "PodState", 1)),
	}}
	if _, err := NewLoader(store).Load(context.Background(), JobReference{}); err == nil || !strings.Contains(err.Error(), "no E2ETest intervals") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoaderAcceptsExactCombinedLimit(t *testing.T) {
	t.Parallel()
	store := &stubStore{names: []string{"run"}, bodies: map[string][]byte{"run": []byte(testJSON)}}
	loader := NewLoader(store)
	loader.totalLimit = len(testJSON)
	if _, err := loader.Load(context.Background(), JobReference{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoaderRetainsInvalidIntervalsAndContinuesToNextArtifact(t *testing.T) {
	t.Parallel()
	bad := strings.Replace(testJSON, "17:11:13Z", "17:11:08Z", 1)
	store := &stubStore{names: []string{"bad", "good"}, bodies: map[string][]byte{"bad": []byte(bad), "good": []byte(testJSON)}}
	data, err := NewLoader(store).Load(context.Background(), JobReference{})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Runs) != 2 || data.EventCount != 2 || data.TestCount != 2 || data.Runs[0].Items[0].Valid || data.Runs[0].Items[0].Reason != "ends before it starts" || !data.Runs[1].Items[0].Valid {
		t.Fatalf("dataset=%+v", data)
	}
}
