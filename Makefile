.PHONY: build test lint ci clean run explain debug

# Build the main binary
build:
	go build -o bin/igor ./cmd/igor/...

# Run the binary locally on a target path (usage: make run path=test/fixtures or defaults to .)
path ?= .
filter ?=
run: build
	./bin/igor $(path)

# Run semantic explanation diagnostics on a target path (usage: make explain path=test/fixtures filter=SuperService)
explain: build
	./bin/igor explain $(path) $(filter)

# Alias to explain subcommand for debugging
debug: explain

# Run all tests
test:
	go test ./... -v

# Run the linter locally
# Note: If this fails due to a Go compiler version mismatch, use 'make docker-lint' or reinstall locally with:
# go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.5
lint:
	@golangci-lint run ./... || (printf "\n⚠️  \033[33mLocal lint failed!\033[0m\n👉 If this is a Go version mismatch, use \033[36mmake docker-lint\033[0m or reinstall the linter locally via:\n   \033[32mgo install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.5\033[0m\n\n" && exit 1)

# Run all CI checks locally
ci: build test lint

# Clean the build directory
clean:
	rm -rf bin/

# --- Docker Development Targets ---

.PHONY: docker-build docker-test docker-lint docker-ci docker-explain docker-debug

# Helper to run commands in the container with Go and golangci-lint caches mounted
DOCKER_RUN = docker run --rm \
	-v $(shell cd .. && pwd):$(shell cd .. && pwd) \
	-v igor-go-cache:/root/.cache/go-build \
	-v igor-go-mod:/go/pkg/mod \
	-v igor-golangci-cache:/root/.cache/golangci-lint \
	-w $(shell pwd) \
	igor-dev

# Build the development Docker image
docker-build:
	docker build -t igor-dev -f Dockerfile.dev .

# Run the test suite within Docker
docker-test: docker-build
	$(DOCKER_RUN) make test

# Run the Go linter within Docker
docker-lint: docker-build
	$(DOCKER_RUN) make lint

# Run full CI validation (build, test, lint) within Docker
docker-ci: docker-build
	$(DOCKER_RUN) make ci

# Run the binary within Docker on a target path (usage: make docker-run path=test/fixtures)
docker-run: docker-build
	$(DOCKER_RUN) make run path=$(path)

# Run semantic explanation diagnostics within Docker (usage: make docker-explain path=test/fixtures filter=SuperService)
docker-explain: docker-build
	$(DOCKER_RUN) make explain path=$(path) filter=$(filter)

# Alias to docker-explain subcommand for debugging inside Docker
docker-debug: docker-explain


