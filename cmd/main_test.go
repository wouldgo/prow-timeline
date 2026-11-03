package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"testing"
)

func TestRunHelp(t *testing.T) {
	t.Parallel()
	if err := run(context.Background(), []string{"-help"}, io.Discard, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error = %v, want flag.ErrHelp", err)
	}
}
