// Package cli implements command-line options and report generation.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wouldgo/prow-timeline/pkg/fetch"
	"github.com/wouldgo/prow-timeline/pkg/timeline"
)

// Loader supplies a complete dataset for a validated job reference.
type Loader interface {
	Load(context.Context, fetch.JobReference) (timeline.Dataset, error)
}

// Command coordinates dataset loading, rendering and output for one invocation.
type Command struct {
	loader    Loader
	render    func(timeline.Dataset) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
}

// New returns a command that renders offline HTML and writes it to disk.
func New(loader Loader) *Command {
	return &Command{loader: loader, render: timeline.Render, writeFile: os.WriteFile}
}

// options contains the arguments for one command invocation.
type options struct {
	output  string
	timeout time.Duration
	jobURL  string
}

// parseOptions checks flags and requires one job URL.
func parseOptions(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("prow-timeline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "timeline.html", "self-contained HTML output file")
	timeout := flags.Duration("timeout", 5*time.Minute, "total discovery and download timeout")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 1 || *timeout <= 0 {
		return options{}, errors.New("usage: prow-timeline [-output timeline.html] [-timeout 5m] PROW_JOB_URL (positive timeout required)")
	}
	return options{output: *output, timeout: *timeout, jobURL: flags.Arg(0)}, nil
}

// Run parses args, generates a report and writes a summary to stdout.
func (c *Command) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	job, err := fetch.ParseJobURL(opts.jobURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	data, err := c.loader.Load(ctx, job)
	if err != nil {
		return err
	}
	page, err := c.render(data)
	if err != nil {
		return err
	}
	if err := c.writeFile(opts.output, page, 0600); err != nil {
		return fmt.Errorf("write HTML: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "Wrote %s: %d step/run artifacts, %d intervals, %d tests\n", opts.output, len(data.Runs), data.EventCount, data.TestCount)
	return err
}
