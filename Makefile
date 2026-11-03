# This file provides build, test, and formatting commands.
GO ?= go
GOFMT ?= gofmt
NODE ?= node
BINARY ?= prow-timeline
COVERAGE_FILE ?= coverage.out
COVERAGE_HTML ?= coverage.html

.DEFAULT_GOAL := build
.DELETE_ON_ERROR:

.PHONY: help all build test test-go test-js test-race vet fmt fmt-check check coverage coverage-html clean

help: ## List the available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-z][a-z-]*:.*## / {printf "  %-16s %s\n", $$1, $$2}' Makefile

all: check test-race build ## Run all checks and build the program.

build: ## Build the command.
	"$(GO)" build -o "$(BINARY)" ./cmd

test: test-go test-js ## Run the Go and JavaScript tests.

test-go: ## Run the Go tests.
	"$(GO)" test ./...

test-js: ## Run the JavaScript tests.
	cd pkg/timeline && "$(NODE)" --test *_test.js

test-race: ## Run the Go tests with the race detector.
	"$(GO)" test -race ./...

vet: ## Check the Go code with go vet.
	"$(GO)" vet ./...

fmt: ## Format the Go code.
	"$(GOFMT)" -w cmd internal pkg

fmt-check: ## Check Go formatting without changing files.
	@unformatted=$$("$(GOFMT)" -l cmd internal pkg) || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		printf '%s\n' 'Run make fmt to format these files.'; \
		exit 1; \
	fi

check: fmt-check vet test ## Check formatting and code, and run all tests.

coverage: ## Write the Go coverage data.
	"$(GO)" test -coverprofile="$(COVERAGE_FILE)" ./...

coverage-html: coverage ## Create the Go coverage report.
	"$(GO)" tool cover -html="$(COVERAGE_FILE)" -o "$(COVERAGE_HTML)"

clean: ## Remove the binary and coverage files.
	$(RM) -- "$(BINARY)" "$(COVERAGE_FILE)" "$(COVERAGE_HTML)"
