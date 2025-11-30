#!/bin/sh
# build.sh - Build unz binaries
#
# Usage:
#   ./build.sh           # Build all binaries to ./bin/
#   ./build.sh install   # Install to $GOPATH/bin
#
# Copyright (c) 2025 haitch <h@ual.fi>
# SPDX-License-Identifier: MIT OR Apache-2.0

set -e

BINDIR="${BINDIR:-./bin}"

build_all() {
    echo "Building unz..."
    mkdir -p "$BINDIR"
    
    echo "  enz       -> $BINDIR/enz"
    go build -o "$BINDIR/enz" ./cmd/enz
    
    echo "  unz       -> $BINDIR/unz"
    go build -o "$BINDIR/unz" ./cmd/unz
    
    echo "  mkdict    -> $BINDIR/mkdict"
    go build -o "$BINDIR/mkdict" ./cmd/mkdict
    
    echo "  benchmark -> $BINDIR/benchmark"
    go build -o "$BINDIR/benchmark" ./cmd/benchmark
    
    echo ""
    echo "Build complete. Binaries in $BINDIR/"
}

install_all() {
    echo "Installing to \$GOPATH/bin..."
    go install ./cmd/enz
    go install ./cmd/unz
    go install ./cmd/mkdict
    go install ./cmd/benchmark
    echo "Install complete."
}

case "${1:-}" in
    install)
        install_all
        ;;
    clean)
        echo "Cleaning $BINDIR/..."
        rm -rf "$BINDIR"
        echo "Clean complete."
        ;;
    *)
        build_all
        ;;
esac
