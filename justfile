set shell := ["zsh", "-cu"]
set positional-arguments := true

default:
    @just --list

build:
    mkdir -p bin
    go build -o bin/recon ./cmd/recon

run *args:
    go run ./cmd/recon {{args}}

install:
    go install ./cmd/recon

fmt:
    go fmt ./...

test:
    go test ./...

test-race:
    go test -race ./...

cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

cover-html:
    go tool cover -html=coverage.out

init:
    go run ./cmd/recon init

sync:
    go run ./cmd/recon sync

orient *args:
    go run ./cmd/recon orient {{args}}

find *args:
    go run ./cmd/recon find {{args}}

decide *args:
    go run ./cmd/recon decide {{args}}

recall *args:
    go run ./cmd/recon recall {{args}}

clean:
    rm -rf bin
    rm -f coverage.out

db-reset:
    rm -f .recon/recon.db
