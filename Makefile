.PHONY: build test lint ci clean run

# Build the main binary
build:
	go build -o bin/igor ./cmd/igor/...

# Run the binary locally on a target path (usage: make run path=test/fixtures or defaults to .)
path ?= .
run: build
	./bin/igor $(path)

# Run all tests
test:
	go test ./... -v

# Run the linter
lint:
	golangci-lint run ./...

# Run all CI checks locally
ci: build test lint

# Clean the build directory
clean:
	rm -rf bin/
