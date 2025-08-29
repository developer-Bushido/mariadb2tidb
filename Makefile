.PHONY: build test clean

build:
	mkdir -p bin
	go build -o bin/mariadb2tidb ./cmd/mariadb2tidb

test:
	go test ./... -count=1

clean:
	rm -rf bin/

install-deps:
	go mod download
	go mod tidy

lint:
	golangci-lint run

bench:
	go test -bench=. ./... 