// Command prow-timeline creates an HTML report from a public Prow job.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/wouldgo/prow-timeline/internal/cli"
	"github.com/wouldgo/prow-timeline/pkg/fetch"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return cli.New(fetch.NewFetcher()).Run(ctx, args, stdout, stderr)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
