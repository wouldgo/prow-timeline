// Package fetch loads timeline datasets from public Prow job artifacts.
package fetch

import (
	"context"

	"github.com/wouldgo/prow-timeline/pkg/timeline"
)

// Fetcher loads datasets from public Google Cloud Storage.
// It composes HTTP reading, artifact discovery and dataset loading.
type Fetcher struct {
	loader *Loader
}

// NewFetcher returns a fetcher with bounded responses, a 60-second request
// timeout and redirects disabled.
func NewFetcher() *Fetcher {
	store := &gcsStore{reader: newHTTPReader(), base: "https://storage.googleapis.com"}
	return &Fetcher{loader: NewLoader(store)}
}

// Load reads and validates all supported timeline artifacts for job.
func (f *Fetcher) Load(ctx context.Context, job JobReference) (timeline.Dataset, error) {
	return f.loader.Load(ctx, job)
}
