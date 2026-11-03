// This file combines validated artifacts into one dataset.

package fetch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wouldgo/prow-timeline/pkg/timeline"
)

const maxTotal = 256 << 20

// Store discovers and downloads supported timeline artifacts for a job.
type Store interface {
	// Discover returns supported artifact names in discovery order.
	Discover(context.Context, JobReference) ([]string, error)
	// Download returns the decoded bytes of one discovered artifact.
	Download(context.Context, JobReference, string) ([]byte, error)
}

// Loader validates timeline artifacts and assembles a complete dataset.
// A failed load returns an empty dataset, never partial results.
type Loader struct {
	store      Store
	totalLimit int
}

// NewLoader returns a loader backed by store.
func NewLoader(store Store) *Loader {
	return &Loader{store: store, totalLimit: maxTotal}
}

// Load reads all supported artifacts in discovery order and validates the dataset.
func (l *Loader) Load(ctx context.Context, job JobReference) (timeline.Dataset, error) {
	names, err := l.store.Discover(ctx, job)
	if err != nil {
		return timeline.Dataset{}, err
	}
	data := timeline.Dataset{JobURL: job.URL}
	total := 0
	totalLimit := l.totalLimit
	if totalLimit <= 0 {
		totalLimit = maxTotal
	}
	for _, name := range names {
		body, err := l.store.Download(ctx, job, name)
		if err != nil {
			return timeline.Dataset{}, fmt.Errorf("download %s: %w", name, err)
		}
		total += len(body)
		if total > totalLimit {
			return timeline.Dataset{}, fmt.Errorf("combined artifacts exceed %d decoded bytes", totalLimit)
		}
		run, err := timeline.ParseTimeline(body, strings.TrimPrefix(name, job.Prefix))
		if err != nil {
			return timeline.Dataset{}, fmt.Errorf("artifact %s: %w", name, err)
		}
		data.Runs = append(data.Runs, run)
		data.EventCount += len(run.Items)
		data.TestCount += run.TestCount
	}
	if data.TestCount == 0 {
		return timeline.Dataset{}, errors.New("no E2ETest intervals with locator.keys[e2e-test] in supported artifacts")
	}
	return data, nil
}
