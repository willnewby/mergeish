# Default recipe
default: build

# Build the binary
build:
    go build -o mergeish ./cmd/mergeish

# Run tests
test:
    go test ./...

# Run tests with coverage
test-coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
    go fmt ./...

# Lint code
lint:
    golangci-lint run

# Clean build artifacts
clean:
    rm -f mergeish
    rm -f coverage.out coverage.html
    rm -rf dist/

# Install locally using go install
install:
    go install ./cmd/mergeish

# Build snapshot release (no publish)
snapshot:
    goreleaser release --snapshot --clean

# Build and install locally via goreleaser
release-local:
    goreleaser release --snapshot --clean
    @mkdir -p ~/bin
    @echo "Installing to ~/bin..."
    cp dist/mergeish_`go env GOOS`_`go env GOARCH`*/mergeish ~/bin/

# Full release (requires GITHUB_TOKEN)
release:
    goreleaser release --clean

# Show help
help:
    @just --list
