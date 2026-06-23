.PHONY: build test race cover lint fmt vet clean install dist tidy

BIN     := hv
PKG     := ./cmd/hv
# Version from git tags (e.g. v1.2.3, or v1.2.3-5-gabc123 between tags), "dev" otherwise.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X haven/internal/cli.Version=$(VERSION)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

test:
	go test ./...

# Race detector + uncached: the gate before any release.
race:
	go test -race -count=1 ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint: vet
vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

# Cross-compiled static release binaries for the platforms we support.
dist: clean
	@mkdir -p dist
	@for osarch in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${osarch%/*}; arch=$${osarch#*/}; ext=""; [ "$$os" = windows ] && ext=".exe"; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN)-$(VERSION)-$$os-$$arch$$ext $(PKG) || exit 1; \
	done
	@echo "release binaries in dist/ (version $(VERSION))"

install: build
	install -m 0755 $(BIN) $(HOME)/.local/bin/$(BIN)

clean:
	rm -f $(BIN) coverage.out
	rm -rf dist
	go clean ./...
