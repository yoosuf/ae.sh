SHELL := /bin/bash

BIN := aef

.PHONY: help check fmt fmt-check build build-bin vet test test-race validate e2e e2e-worktree clean

.DEFAULT_GOAL := help

help:
	@printf '%s\n' \
	  'AEF local verification targets:' \
	  '  make fmt        format Go sources in cmd/ and internal/' \
	  '  make fmt-check  fail if Go sources are not formatted' \
	  '  make build      compile all packages' \
	  '  make build-bin  build ./aef' \
	  '  make vet        run go vet ./...' \
	  '  make test       run go test ./...' \
	  '  make test-race  run go test -race ./...' \
	  '  make validate   validate skills, workflows, policies, and config' \
	  '  make e2e        run the credential-free offline end-to-end script' \
	  '  make e2e-worktree  run the detached-worktree E2E in a throwaway git repo' \
	  '  make check      run all local checks above (except fmt/build-bin)' \
	  '  make clean      remove the local binary'

fmt:
	gofmt -w cmd internal

fmt-check:
	@files="$$(gofmt -l cmd internal)"; \
	if [ -n "$$files" ]; then \
	  echo "unformatted Go files:" >&2; \
	  echo "$$files" >&2; \
	  exit 1; \
	fi

build:
	go build ./...

build-bin:
	go build -o $(BIN) ./cmd/aef

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

validate:
	scripts/check-schemas.sh

e2e:
	scripts/e2e-offline.sh

e2e-worktree:
	scripts/e2e-worktree.sh

check: fmt-check build vet test test-race validate e2e e2e-worktree

clean:
	rm -f $(BIN)
