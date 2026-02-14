set shell := ["zsh", "-cu"]
set positional-arguments := true

# Show available recipes and their descriptions.
[group('meta')]
default:
    @just --list --unsorted

# Build the recon binary into ./bin/recon.
[group('build')]
build:
    mkdir -p bin
    go build -o bin/recon ./cmd/recon

# Install recon to GOPATH/bin.
[group('build')]
install:
    go install ./cmd/recon

# Run recon via go run, forwarding all args.
[group('build')]
run *args:
    go run ./cmd/recon {{args}}

# Generate coverage.out and print function coverage summary.
[group('quality')]
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

# Open HTML coverage report from coverage.out.
[group('quality')]
cover-html:
    go tool cover -html=coverage.out

# Format all Go packages.
[group('quality')]
fmt:
    go fmt ./...

# Run the full test suite.
[group('quality')]
test:
    go test ./...

# Run tests with the race detector enabled.
[group('quality')]
test-race:
    go test -race ./...

# Record a decision with checks/evidence flags.
[group('workflow')]
decide *args:
    go run ./cmd/recon decide {{args}}

# Search indexed symbols/files/imports.
[group('workflow')]
find *args:
    go run ./cmd/recon find {{args}}

# Initialize local recon database and schema.
[group('workflow')]
init:
    go run ./cmd/recon init

# Show orient output (status + suggested next actions).
[group('workflow')]
orient *args:
    go run ./cmd/recon orient {{args}}

# Recall previously recorded decisions.
[group('workflow')]
recall *args:
    go run ./cmd/recon recall {{args}}

# Index and sync current repository state.
[group('workflow')]
sync:
    go run ./cmd/recon sync

# Remove local build and coverage artifacts.
[group('cleanup')]
clean:
    rm -rf bin
    rm -f coverage.out

# Delete the local recon SQLite database.
[group('cleanup')]
db-reset:
    rm -f .recon/recon.db
