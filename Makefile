PREFIX  ?= $(HOME)/.local/bin
VERSION ?= dev
LDFLAGS  = -X github.com/DrBopoH/sky-cli/internal/version.Version=$(VERSION)

.PHONY: test install

test:
	cd project && go vet ./... && go test ./...

install:
	cd project && go build -ldflags "$(LDFLAGS)" -o $(PREFIX)/sky .
