BINARY := containershell
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildDate=$(DATE)"

.PHONY: build test lint clean install completion

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/containershell/

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

install: build
	install -m 755 $(BINARY) /usr/local/bin/

completion:
	./$(BINARY) completion bash > /etc/bash_completion.d/$(BINARY) 2>/dev/null || true
	./$(BINARY) completion zsh > /usr/local/share/zsh/site-functions/_$(BINARY) 2>/dev/null || true
