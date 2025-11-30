#!/bin/sh
# benchmark.sh - Run compression benchmarks
#
# Usage:
#   ./benchmark.sh                    # Run with default sizes
#   ./benchmark.sh -sizes 2,8,32,128  # Custom sizes (KB)
#   ./benchmark.sh -o ./reports       # Custom output directory
#   ./benchmark.sh -quick             # Quick run (2,16,128,1024 KB)
#   ./benchmark.sh -full              # Full run (2-2048 KB, all sizes)
#
# Copyright (c) 2025 haitch <h@ual.fi>
# SPDX-License-Identifier: MIT OR Apache-2.0

set -e

# Defaults
OUTPUT_DIR="./benchmarks"
SIZES="2,4,8,16,32,64,128,256,512,1024,2048"

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -sizes|--sizes)
            SIZES="$2"
            shift 2
            ;;
        -quick|--quick)
            SIZES="2,16,128,1024"
            shift
            ;;
        -full|--full)
            SIZES="2,4,8,16,32,64,128,256,512,1024,2048"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  -o, --output DIR   Output directory (default: ./benchmarks)"
            echo "  -sizes LIST        Comma-separated sizes in KB"
            echo "  -quick             Quick run: 2,16,128,1024 KB"
            echo "  -full              Full run: 2-2048 KB (default)"
            echo "  -h, --help         Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                          # Full benchmark"
            echo "  $0 -quick                   # Quick test"
            echo "  $0 -sizes 8,64,512          # Custom sizes"
            echo "  $0 -o ./reports -quick      # Quick to custom dir"
            echo ""
            echo "Output:"
            echo "  report.html   Interactive HTML report with charts"
            echo "  report.json   Machine-readable JSON data"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Check dependencies
if ! command -v zip >/dev/null 2>&1; then
    echo "Error: 'zip' command not found. Please install zip." >&2
    exit 1
fi

if ! command -v gzip >/dev/null 2>&1; then
    echo "Error: 'gzip' command not found. Please install gzip." >&2
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "Compression Benchmark"
echo "====================="
echo ""
echo "Output:    $OUTPUT_DIR"
echo "Sizes:     $SIZES KB"
echo ""

# Build benchmark tool if needed
if [ ! -x ./bin/benchmark ]; then
    echo "Building benchmark tool..."
    go build -o ./bin/benchmark ./cmd/benchmark
    echo ""
fi

# Run benchmark
./bin/benchmark -o "$OUTPUT_DIR" -sizes "$SIZES"

echo ""
echo "Results:"
echo "  HTML: $OUTPUT_DIR/report.html"
echo "  JSON: $OUTPUT_DIR/report.json"
echo ""
echo "Open $OUTPUT_DIR/report.html in a browser to view interactive charts."
