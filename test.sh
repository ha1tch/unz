#!/bin/sh
# test.sh - Run unz test suite
#
# Usage:
#   ./test.sh              # Run all tests
#   ./test.sh -v           # Verbose output
#   ./test.sh -cover       # With coverage report
#   ./test.sh -race        # With race detector
#   ./test.sh pkg/compress # Test specific package
#
# Copyright (c) 2025 haitch <h@ual.fi>
# SPDX-License-Identifier: MIT OR Apache-2.0

set -e

# Default flags
VERBOSE=""
COVER=""
RACE=""
PACKAGE="./..."

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -cover|--cover)
            COVER="-cover"
            shift
            ;;
        -race|--race)
            RACE="-race"
            shift
            ;;
        -coverprofile)
            COVER="-coverprofile=$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options] [package]"
            echo ""
            echo "Options:"
            echo "  -v, --verbose     Verbose test output"
            echo "  -cover            Show coverage summary"
            echo "  -coverprofile F   Write coverage to file F"
            echo "  -race             Enable race detector"
            echo "  -h, --help        Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                     # Run all tests"
            echo "  $0 -v -cover           # Verbose with coverage"
            echo "  $0 ./pkg/compress      # Test one package"
            echo "  $0 -coverprofile c.out # Generate coverage file"
            exit 0
            ;;
        -*)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
        *)
            PACKAGE="$1"
            shift
            ;;
    esac
done

echo "Running tests..."
echo ""

# Run tests
# shellcheck disable=SC2086
go test $VERBOSE $COVER $RACE $PACKAGE

# If coverage profile requested, show summary
if echo "$COVER" | grep -q "coverprofile"; then
    PROFILE=$(echo "$COVER" | cut -d= -f2)
    if [ -f "$PROFILE" ]; then
        echo ""
        echo "Coverage profile written to: $PROFILE"
        echo ""
        echo "To view HTML report:"
        echo "  go tool cover -html=$PROFILE -o coverage.html"
    fi
fi

echo ""
echo "All tests passed."
