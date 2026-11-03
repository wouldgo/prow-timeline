package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wouldgo/prow-timeline/pkg/fetch"
	"github.com/wouldgo/prow-timeline/pkg/timeline"
)

const jobURL = "https://prow.ci.openshift.org/view/gs/bucket/logs/job/123"

type loaderFunc func(context.Context, fetch.JobReference) (timeline.Dataset, error)

func (f loaderFunc) Load(ctx context.Context, job fetch.JobReference) (timeline.Dataset, error) {
	return f(ctx, job)
}

func TestParseOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want options
	}{
		{"defaults", []string{jobURL}, options{"timeline.html", 5 * time.Minute, jobURL}},
		{"custom", []string{"-output", "report.html", "-timeout", "3s", jobURL}, options{"report.html", 3 * time.Second, jobURL}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseOptions(tc.args, io.Discard)
			if err != nil || got != tc.want {
				t.Fatalf("options = %+v, error = %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestCommandRejectsArgumentsBeforeLoading(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		nil, {"-timeout", "0s", jobURL}, {"-timeout", "-1s", jobURL},
		{"-unknown"}, {"https://example.com"}, {jobURL, jobURL},
	} {
		command := New(loaderFunc(func(context.Context, fetch.JobReference) (timeline.Dataset, error) {
			t.Fatal("loader called for invalid arguments")
			return timeline.Dataset{}, nil
		}))
		if err := command.Run(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Fatalf("accepted args %v", args)
		}
	}
}

func TestCommandCoordinatesReportGeneration(t *testing.T) {
	t.Parallel()
	data := timeline.Dataset{JobURL: jobURL, Runs: []timeline.Timeline{{Name: "run"}}, EventCount: 7, TestCount: 3}
	var calls []string
	var requestContext context.Context
	command := New(loaderFunc(func(ctx context.Context, job fetch.JobReference) (timeline.Dataset, error) {
		calls = append(calls, "load")
		requestContext = ctx
		if job.URL != jobURL || job.Bucket != "bucket" || job.Prefix != "logs/job/123/artifacts/" {
			t.Fatalf("job = %+v", job)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 3*time.Second {
			t.Fatalf("deadline = %v, exists = %v", deadline, ok)
		}
		return data, nil
	}))
	command.render = func(got timeline.Dataset) ([]byte, error) {
		calls = append(calls, "render")
		if !reflect.DeepEqual(got, data) {
			t.Fatalf("rendered dataset = %+v", got)
		}
		return []byte("report"), nil
	}
	command.writeFile = func(name string, body []byte, mode os.FileMode) error {
		calls = append(calls, "write")
		if name != "report.html" || string(body) != "report" || mode != 0600 {
			t.Fatalf("write = %q, %q, %o", name, body, mode)
		}
		return nil
	}
	var stdout bytes.Buffer
	err := command.Run(context.Background(), []string{"-output", "report.html", "-timeout", "3s", jobURL}, &stdout, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"load", "render", "write"}) {
		t.Fatalf("calls = %v", calls)
	}
	if stdout.String() != "Wrote report.html: 1 step/run artifacts, 7 intervals, 3 tests\n" {
		t.Fatalf("summary = %q", stdout.String())
	}
	if requestContext.Err() != context.Canceled {
		t.Fatal("request context was not released")
	}
}

func TestCommandStopsAtFailedStage(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("stage failed")
	for _, stage := range []string{"load", "render", "write", "summary"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			var calls []string
			command := New(loaderFunc(func(context.Context, fetch.JobReference) (timeline.Dataset, error) {
				calls = append(calls, "load")
				if stage == "load" {
					return timeline.Dataset{}, sentinel
				}
				return timeline.Dataset{}, nil
			}))
			command.render = func(timeline.Dataset) ([]byte, error) {
				calls = append(calls, "render")
				if stage == "render" {
					return nil, sentinel
				}
				return nil, nil
			}
			command.writeFile = func(string, []byte, os.FileMode) error {
				calls = append(calls, "write")
				if stage == "write" {
					return sentinel
				}
				return nil
			}
			var stdout io.Writer = io.Discard
			if stage == "summary" {
				stdout = failingWriter{sentinel}
			}
			err := command.Run(context.Background(), []string{jobURL}, stdout, io.Discard)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v", err)
			}
			want := map[string][]string{
				"load": {"load"}, "render": {"load", "render"},
				"write": {"load", "render", "write"}, "summary": {"load", "render", "write"},
			}[stage]
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls = %v, want %v", calls, want)
			}
			if stage == "write" && !strings.Contains(err.Error(), "write HTML:") {
				t.Fatalf("missing output error context: %v", err)
			}
		})
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCommandPreservesParentCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := New(loaderFunc(func(ctx context.Context, _ fetch.JobReference) (timeline.Dataset, error) {
		return timeline.Dataset{}, ctx.Err()
	}))
	if err := command.Run(ctx, []string{jobURL}, io.Discard, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandWritesOfflineReport(t *testing.T) {
	t.Parallel()
	command := New(loaderFunc(func(context.Context, fetch.JobReference) (timeline.Dataset, error) {
		return timeline.Dataset{Runs: []timeline.Timeline{{Name: "example"}}}, nil
	}))
	output := filepath.Join(t.TempDir(), "report.html")
	if err := command.Run(context.Background(), []string{"-output", output, jobURL}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("<!doctype html>")) || bytes.Contains(body, []byte("DATA_PLACEHOLDER")) {
		t.Fatal("invalid offline report")
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
}

func TestCommandReportsFileWriteFailure(t *testing.T) {
	t.Parallel()
	command := New(loaderFunc(func(context.Context, fetch.JobReference) (timeline.Dataset, error) {
		return timeline.Dataset{}, nil
	}))
	err := command.Run(context.Background(), []string{"-output", t.TempDir(), jobURL}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "write HTML:") {
		t.Fatalf("error = %v", err)
	}
}
