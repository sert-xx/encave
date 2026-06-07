VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/sert-xx/encave/internal/cli.Version=$(VERSION)

.PHONY: build test vet fmt install clean

build:
	go build -ldflags "$(LDFLAGS)" -o encave .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -f encave
	go clean
