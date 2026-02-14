set shell := ["zsh", "-cu"]
set positional-arguments := true

[group('meta')]
# Show available recipes and their descriptions.
default:
    @just --list --unsorted

[group('build')]
# Build the recon binary into ./bin/recon.
build:
    mkdir -p bin
    go build -o bin/recon ./cmd/recon

[group('build')]
# Install recon to GOPATH/bin.
install:
    go install ./cmd/recon

[group('build')]
# Run recon via go run, forwarding all args.
run *args:
    go run ./cmd/recon {{args}}

[group('quality')]
# Generate coverage.out and print function coverage summary.
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

[group('quality')]
# Open HTML coverage report from coverage.out.
cover-html:
    go tool cover -html=coverage.out

[group('quality')]
# Format all Go packages.
fmt:
    go fmt ./...

[group('quality')]
# Run the full test suite.
test:
    go test ./...

[group('quality')]
# Run tests with the race detector enabled.
test-race:
    go test -race ./...

[group('workflow')]
# Record a decision with checks/evidence flags.
decide *args:
    go run ./cmd/recon decide {{args}}

[group('workflow')]
# Search indexed symbols/files/imports.
find *args:
    go run ./cmd/recon find {{args}}

[group('workflow')]
# Initialize local recon database and schema.
init:
    go run ./cmd/recon init

[group('workflow')]
# Show orient output (status + suggested next actions).
orient *args:
    go run ./cmd/recon orient {{args}}

[group('workflow')]
# Recall previously recorded decisions.
recall *args:
    go run ./cmd/recon recall {{args}}

[group('workflow')]
# Index and sync current repository state.
sync:
    go run ./cmd/recon sync

[group('cleanup')]
# Remove local build and coverage artifacts.
clean:
    rm -rf bin
    rm -f coverage.out

[group('cleanup')]
# Delete the local recon SQLite database.
db-reset:
    rm -f .recon/recon.db
