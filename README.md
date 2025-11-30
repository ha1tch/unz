# unz

ZIP-compatible compression with adaptive BPE methods.

## Overview

**unz** creates standard PKZIP format files. It adds two proprietary methods optimized for source code:

| Method | Code | Pipeline | Best for |
|--------|------|----------|----------|
| Stored | 0 | None | Incompressible data |
| Deflate | 8 | LZ77+Huffman | Large files (>64KB) |
| **Bpelate** | 86 ('V') | BPE → DEFLATE | **Source code, small-medium files** |
| Unzlate | 85 ('U') | BPE → ANS | Reserved (high overhead) |

**Bpelate** is the key innovation: BPE tokenization as a pre-processor for DEFLATE.

## How Bpelate Works

1. **Tokenize** - Convert source code to token IDs using language-specific vocabulary
2. **Varint encode** - Compact representation of token stream
3. **DEFLATE** - Standard compression catches repeated token sequences

The vocabulary metadata is stored in ZIP extra field 0x554E (4 bytes).

## Benchmark Results

Comprehensive benchmarks across 21 content types (7 natural languages, 7 programming languages, 4 structured data formats, 3 markup languages) at 11 file sizes (2KB to 2MB):

| Metric | Value |
|--------|-------|
| Total tests | 231 |
| **enz win rate** | **100%** |
| **Avg improvement vs zip** | **+6.6%** |

### By Category

| Category | Avg Improvement |
|----------|-----------------|
| Programming languages | **+10.8%** |
| Natural languages | +5.0% |
| Markup | +4.9% |
| Structured data | +3.3% |

### By File Size

| Size | Improvement | Size | Improvement |
|------|-------------|------|-------------|
| 2 KB | +12.5% | 128 KB | +4.5% |
| 8 KB | +9.0% | 512 KB | +4.2% |
| 32 KB | +6.2% | 2048 KB | +4.2% |

### Best Results

- **Python code**: +27-29% improvement (BPE captures `def `, `self.`, indentation)
- **Go code**: +27% on small files (BPE captures `func `, `if err != nil`, `:=`)

Full interactive report: [benchmarks/report.html](benchmarks/report.html)

## Installation
```bash
go install github.com/ha1tch/unz/cmd/enz@latest
go install github.com/ha1tch/unz/cmd/unz@latest
```

Or build locally:
```bash
./build.sh              # Build to ./bin/
./build.sh install      # Install to $GOPATH/bin
```

## Usage
```bash
# Compress (auto-selects best method)
enz archive.zip file.go

# Extract
unz archive.zip

# List with details
unz -v archive.zip
```

## Auto-Selection Logic

The compressor automatically detects content type and selects the best method:
```
if detected as code:
    try DEFLATE, BPELATE with language-specific vocabulary
    pick smallest
else if natural language text:
    try DEFLATE, BPELATE with text vocabulary
    pick smallest
else if high entropy (random/encrypted):
    use STORED
else:
    use DEFLATE
```

## Supported Languages

### Content Detection

The detector identifies 39 content types across four categories:

#### Natural Languages (14)

| Language | Detection Method |
|----------|------------------|
| English | Common words: "the", "and", "is", "to" |
| Spanish | "el", "la", "de", "que", "ñ", "ción" |
| French | "le", "la", "les", "est", "ç", "œ" |
| Portuguese | "o", "a", "de", "que", "ção", "ã" |
| German | "der", "die", "das", "und", "ß", "ü" |
| Italian | "il", "la", "di", "che", "è" |
| Dutch | "de", "het", "een", "van", "ij" |
| Chinese | Unicode range 0x4E00-0x9FFF |
| Japanese | Hiragana/Katakana 0x3040-0x30FF |
| Arabic | Unicode range 0x0600-0x06FF |
| Russian | Cyrillic 0x0400-0x04FF |
| Hindi | Devanagari 0x0900-0x097F |
| Bengali | Unicode range 0x0980-0x09FF |
| Indonesian | "yang", "dan", "di", "dengan" |

#### Programming Languages (12)

| Language | Detection Patterns |
|----------|-------------------|
| Go | `package`, `func`, `:=`, `if err != nil`, `defer` |
| Python | `def`, `self.`, `__init__`, `import`, `elif` |
| JavaScript | `const`, `let`, `=>`, `require(`, `useState` |
| Java | `public class`, `System.out`, `@Override` |
| C | `#include <`, `printf(`, `malloc(`, `sizeof(` |
| C++ | `std::`, `cout <<`, `namespace`, `template<` |
| C# | `using System`, `Console.WriteLine`, `get;` `set;` |
| Ruby | `def`...`end`, `attr_accessor`, `.each`, `do \|` |
| Rust | `fn`, `println!`, `use std::`, `impl`, `match` |
| PHP | `<?php`, `echo`, `$this->`, `namespace` |
| Swift | `import Foundation`, `guard let`, `@IBOutlet` |
| Kotlin | `fun`, `val`, `var`, `?.`, `data class` |

#### Structured Data Formats (6)

| Format | Detection |
|--------|-----------|
| JSON | Starts with `{` or `[`, contains `":` |
| XML | Starts with `<`, contains `<?xml` or tag structure |
| YAML | `key: value` patterns, `---` header |
| CSV | Comma-separated with consistent columns |
| TOML | `[section]` headers with `key = value` |
| INI | `[section]` headers with `key=value` |

#### Markup Languages (7)

| Format | Detection |
|--------|-----------|
| HTML | `<!doctype html>`, `<html>`, `<head>`, `<body>` |
| Markdown | `#` headers, ` ``` ` code blocks, `[links](url)` |
| LaTeX | `\documentclass`, `\begin{document}`, `\section{` |
| RTF | Starts with `{\rtf` |
| reStructuredText | `===` underlines, `.. directive::` |
| AsciiDoc | `= Title`, `== Section`, `:toc:` |
| Org-mode | `* heading`, `#+TITLE`, `#+BEGIN_SRC` |

## VocabInfo Extra Field (0x554E)

Bpelate archives include a 4-byte vocabulary descriptor:
```
Offset  Size  Field
0       1     Natural language (human language for comments/docs)
1       1     Programming language
2       1     Structured data format
3       1     Markup language
```

### Natural Languages

| Code | Language | Coverage |
|------|----------|----------|
| 0x00 | Unspecified | - |
| 0x01 | English | 50% |
| 0x02 | Spanish | |
| 0x03 | French | |
| 0x04 | Portuguese | |
| 0x05 | German | |
| 0x06 | Italian | |
| 0x07 | Dutch | |
| 0x08 | Chinese | 67% |
| 0x09 | Arabic | 72% |
| 0x0A | Hindi | 90% |
| 0x0B | Indonesian/Malay | 94% |
| 0x0C | Bengali | 96% |
| 0x0D | Russian | 97% |
| 0x0E | Japanese | 98% |

### Programming Languages

| Code | Language |
|------|----------|
| 0x00 | None |
| 0x01 | Go |
| 0x02 | Python |
| 0x03 | JavaScript/TypeScript |
| 0x04 | Java |
| 0x05 | C |
| 0x06 | C++ |
| 0x07 | C# |
| 0x08 | Ruby |
| 0x09 | Rust |
| 0x0A | PHP |
| 0x0B | Swift |
| 0x0C | Kotlin |

### Structured Data Formats

| Code | Format |
|------|--------|
| 0x00 | None |
| 0x01 | JSON |
| 0x02 | XML |
| 0x03 | YAML |
| 0x04 | CSV |
| 0x05 | TOML |
| 0x06 | INI |

### Markup Languages

| Code | Format |
|------|--------|
| 0x00 | None |
| 0x01 | HTML |
| 0x02 | Markdown |
| 0x03 | LaTeX |
| 0x04 | RTF |
| 0x05 | reStructuredText |
| 0x06 | AsciiDoc |
| 0x07 | Org-mode |

## Standard Tool Compatibility

Standard tools show:
- `unzip -l`: Lists files correctly
- `unzip -v`: Shows "Unk:085" or "Unk:086"
- Extraction: "unsupported compression method"

## Vocabularies

Pre-trained BPE vocabularies (~1750 tokens each):

| Vocabulary | Tokens | Trained on |
|------------|--------|------------|
| Text | 756 | English prose |
| Go | 1756 | Go source code |
| Python | 1746 | Python source |
| JavaScript | 1756 | JS/TS source |

### Custom Vocabularies
```bash
mkdict -n 2000 corpus/*.txt > pkg/vocab/custom_tokens.go
go build ./...
```

## Why Bpelate Beats DEFLATE

DEFLATE uses LZ77 (backreferences) + Huffman coding. It finds repeated byte sequences.

Bpelate first converts code to tokens:
```go
"if err != nil" → [tok_if, tok_err, tok_neq, tok_nil]
```

This creates:
1. **Shorter sequences** - Fewer bytes to compress
2. **More repetition** - Token IDs repeat more than raw bytes
3. **Better entropy** - DEFLATE's Huffman coding works on a smaller alphabet

The overhead is the vocabulary (shared, not embedded) and 4 bytes for VocabInfo.

## Project Structure
```
unz/
├── cmd/
│   ├── enz/          # Compression CLI
│   ├── unz/          # Decompression CLI  
│   ├── mkdict/       # Vocabulary generator
│   └── benchmark/    # Benchmark report generator
├── pkg/
│   ├── ans/          # rANS entropy coding
│   ├── bpe/          # Byte Pair Encoding
│   ├── compress/     # ZIP format, method selection
│   ├── detect/       # Content type detection
│   └── vocab/        # Embedded vocabularies
├── benchmarks/       # Benchmark reports
│   ├── report.html   # Interactive HTML report
│   ├── report.json   # Machine-readable data
│   └── README.md     # Benchmark documentation
├── build.sh          # Build script
├── test.sh           # Test runner
├── benchmark.sh      # Benchmark runner
├── LICENSE           # Dual licence notice
├── LICENSE-MIT       # MIT licence text
└── LICENSE-APACHE    # Apache 2.0 licence text
```

## Scripts
```bash
./build.sh              # Build all binaries to ./bin/
./build.sh install      # Install to $GOPATH/bin
./build.sh clean        # Remove ./bin/

./test.sh               # Run all tests
./test.sh -v            # Verbose output
./test.sh -cover        # With coverage summary
./test.sh -race         # With race detector

./benchmark.sh          # Run full benchmark suite
./benchmark.sh -quick   # Quick run (4 sizes)
./benchmark.sh -sizes 2,8,32  # Custom sizes
```

## Limitations

- Max file size: 4GB (no ZIP64)
- Single-file archives only
- Vocabularies must match between compressor/decompressor

## Dependencies

None. Standard library only.

## Author

Copyright (C) 2025 haitch <h@ual.fi>

## Licence

Dual-licensed under MIT and Apache 2.0. You may choose either licence.

SPDX-License-Identifier: MIT OR Apache-2.0