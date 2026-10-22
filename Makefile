.PHONY: build test clean fmt vet lint vulncheck bench ci install-deps integration integration-up integration-down

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

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

bench:
	go test -bench=. ./...

ci: fmt vet test

# Integration tests against live servers: real MariaDB -> mariadb-dump ->
# mariadb2tidb -> TiDB (unistore), comparing schema and data. Images are
# pinned by digest, like the CI job.
TIDB_IMAGE    ?= pingcap/tidb:v8.5.8@sha256:df168c764bf2dfdb166dc37a5c3b0e210d29d5f3ab2d33317fd0fdf7b32037f5
MARIADB_IMAGE ?= mariadb:11.4@sha256:1292844148b311e4ed4300022a996d39083f415a963e970cf47cad1b3b18e3a6

integration-up:
	docker run -d --rm --name m2t-tidb --memory 2g -p 127.0.0.1:14000:4000 $(TIDB_IMAGE)
	docker run -d --rm --name m2t-mariadb --memory 1g -p 127.0.0.1:13306:3306 -e MARIADB_ALLOW_EMPTY_ROOT_PASSWORD=1 $(MARIADB_IMAGE)

integration:
	M2T_TIDB_DSN='root@tcp(127.0.0.1:14000)/' \
	M2T_MARIADB_DSN='root@tcp(127.0.0.1:13306)/' \
	M2T_MARIADB_DUMP='docker exec m2t-mariadb mariadb-dump -uroot' \
	go test -tags integration ./test/integration -count=1 -v

integration-down:
	docker rm -f m2t-tidb m2t-mariadb
