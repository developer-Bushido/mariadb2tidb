.PHONY: build test clean fmt vet lint ci

# Version metadata (overridable)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/mariadb2tidb ./cmd/mariadb2tidb

test:
	go test ./... -count=1

clean:
	rm -rf bin/

install-deps:
	go mod download
	go mod tidy

fmt:
	gofmt -s -w $(shell find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...

lint:
	golangci-lint run

bench:
	go test -bench=. ./... 

ci: fmt vet test
