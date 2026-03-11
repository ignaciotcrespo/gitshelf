#!/bin/bash
set -e

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")

go build -ldflags "-s -w -X main.version=${VERSION}" -o gitshelf ./cmd/gitshelf
go install -ldflags "-s -w -X main.version=${VERSION}" ./cmd/gitshelf
