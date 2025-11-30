# Compression Benchmarks

This directory contains benchmark results comparing `enz` against standard compression tools.

## Reports

- **report.html** - Interactive HTML report with charts and tables
- **report.json** - Machine-readable JSON data for programmatic analysis

## Running Benchmarks

To regenerate the benchmarks:

```bash
go run ./cmd/benchmark -o ./benchmarks -sizes "2,4,8,16,32,64,128,256,512,1024,2048"
```

### Options

- `-o <dir>` - Output directory for reports (default: current directory)
- `-sizes <list>` - Comma-separated list of file sizes in KB to test

## Summary of Results

| Metric | Value |
|--------|-------|
| Total tests | 231 |
| enz win rate | 100% |
| Average improvement vs zip | +6.6% |

### By Category

| Category | Tests | Avg Improvement |
|----------|-------|-----------------|
| Programming languages | 77 | +10.8% |
| Natural languages | 77 | +5.0% |
| Markup | 33 | +4.9% |
| Structured data | 44 | +3.3% |

### By File Size

| Size | Avg Improvement |
|------|-----------------|
| 2 KB | +12.5% |
| 4 KB | +10.7% |
| 8 KB | +9.0% |
| 16 KB | +7.5% |
| 32 KB | +6.2% |
| 64 KB | +5.2% |
| 128 KB | +4.5% |
| 256 KB | +4.3% |
| 512 KB | +4.2% |
| 1024 KB | +4.2% |
| 2048 KB | +4.2% |

### Method Selection

The compressor automatically selects the best method:

- **Bpelate** (BPE + DEFLATE): Used for 32.5% of tests, primarily small/medium code files
- **Deflate**: Used for 67.5% of tests, primarily large files where LZ77 dictionary is more effective

### Best Results

| Content | Size | Improvement | Method |
|---------|------|-------------|--------|
| Python | 2048 KB | +29.3% | Bpelate |
| Python | 1024 KB | +29.1% | Bpelate |
| Python | 512 KB | +28.6% | Bpelate |
| Go | 2 KB | +27.1% | Bpelate |

## Test Methodology

- All tests compare ZIP-format outputs (enz vs zip -9)
- gzip results included for reference but use different container format
- Content is synthetically generated to match realistic patterns for each type
- Each content type tested at 11 different sizes from 2KB to 2048KB
