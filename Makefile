.PHONY: build test lint ci clean

# Build the main binary
build:
	go build -o bin/igor ./cmd/igor/...

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
