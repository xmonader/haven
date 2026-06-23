.PHONY: build test lint clean install

BIN := hv

build:
	go build -o $(BIN) ./cmd/hv

test:
	go test ./...

lint:
	go vet ./...

install: build
	install -m 0755 $(BIN) $(HOME)/.local/bin/$(BIN)

clean:
	rm -f $(BIN)
	go clean ./...
