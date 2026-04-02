.PHONY: build test lint clean install

GO := go

build:
	$(GO) build ./...

test:
	$(GO) test -race -count=1 ./...

test-v:
	$(GO) test -race -count=1 -v ./...

test-cover:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

lint:
	$(GO) vet ./...

clean:
	rm -f coverage.out coverage.html

install:
	$(GO) install ./cmd/mcpx

example:
	$(GO) run ./examples/basic
