.PHONY: build test lint check clean review

# Build the binary
build:
	go build -o prism .

# Run tests with race detector
test:
	go test -v -race ./...

# Run linter
lint:
	golangci-lint run ./...

# All quality checks
check: lint test build

# Remove build artifacts
clean:
	rm -f prism coverage.out

# Review a PR (usage: make review PR=2 FLAGS="--format md --verbose")
review: build
	CLAUDECODE= ./prism review $(PR) $(FLAGS)
