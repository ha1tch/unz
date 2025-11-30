// Command enz compresses files using adaptive BPE/DEFLATE compression.
//
// Usage matches zip(1):
//
//	enz [-0|-9] [-q] [-v] [-m] [-j] archive.unz file...
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ha1tch/unz/pkg/compress"
	"github.com/ha1tch/unz/pkg/vocab"
)

var (
	level0    = flag.Bool("0", false, "store only (no compression)")
	level9    = flag.Bool("9", false, "best compression (default)")
	quiet     = flag.Bool("q", false, "quiet operation")
	verbose   = flag.Bool("v", false, "verbose operation")
	move      = flag.Bool("m", false, "move into archive (delete input files)")
	junkPaths = flag.Bool("j", false, "junk (don't record) directory names")
	help      = flag.Bool("h", false, "display this help")
)

func main() {
	flag.Usage = usage
	flag.Parse()

	if *help {
		usage()
		os.Exit(0)
	}

	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "enz: missing archive or file arguments")
		fmt.Fprintln(os.Stderr, "Try 'enz -h' for more information.")
		os.Exit(1)
	}

	archivePath := flag.Arg(0)
	if !strings.HasSuffix(archivePath, ".zip") && !strings.HasSuffix(archivePath, ".unz") {
		archivePath += ".zip"
	}

	inputPath := flag.Arg(1)

	// Check input exists
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		fatal("cannot access '%s': %v", inputPath, err)
	}
	if inputInfo.IsDir() {
		fatal("'%s' is a directory (directories not yet supported)", inputPath)
	}

	// Read input
	input, err := os.ReadFile(inputPath)
	if err != nil {
		fatal("cannot read '%s': %v", inputPath, err)
	}

	// Get the name to store
	storedName := inputPath
	if *junkPaths {
		storedName = filepath.Base(inputPath)
	}

	// Use shared vocabulary
	if !*quiet {
		fmt.Fprintf(os.Stderr, "  adding: %s", storedName)
	}

	comp := compress.New(vocab.Default())

	// Compress with file's actual permissions
	start := time.Now()
	var output []byte
	mode := inputInfo.Mode()

	if *level0 {
		output, err = comp.CompressFileAsWithMode(input, storedName, inputInfo.ModTime(), mode, compress.MethodStore)
	} else {
		output, err = comp.CompressFileWithMode(input, storedName, inputInfo.ModTime(), mode)
	}

	if err != nil {
		fatal("compression failed: %v", err)
	}
	elapsed := time.Since(start)

	// Get compression info
	info, _ := compress.GetFileInfo(output)
	ratio := 100 - (float64(info.CompSize) * 100 / float64(info.Size))
	if ratio < 0 {
		ratio = 0
	}

	if !*quiet {
		methodStr := strings.ToLower(info.Method.String())
		if methodStr == "stored" {
			fmt.Fprintf(os.Stderr, " (stored 0%%)\n")
		} else {
			fmt.Fprintf(os.Stderr, " (%s %.0f%%)\n", methodStr, ratio)
		}
	}

	// Write output
	if err := os.WriteFile(archivePath, output, 0644); err != nil {
		fatal("cannot write '%s': %v", archivePath, err)
	}

	// Verbose stats
	if *verbose {
		fmt.Fprintf(os.Stderr, "  %d bytes -> %d bytes (%.1f%%) in %v\n",
			len(input), len(output), float64(len(output))*100/float64(len(input)),
			elapsed.Round(time.Millisecond))
	}

	// Delete input if -m
	if *move {
		os.Remove(inputPath)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: enz [-0|-9] [-qvmj] archive[.zip] file

Compress file into ZIP archive using adaptive BPE/DEFLATE compression.
Output is standard PKZIP format compatible with unzip, WinZip, etc.

Options:
  -0        store only (no compression)
  -9        best compression (default)
  -q        quiet operation
  -v        verbose operation  
  -m        move into archive (delete input file after compression)
  -j        junk directory names (store only the file name)
  -h        display this help

Compression methods:
  Method 0  (Stored)  - no compression
  Method 8  (Deflate) - standard ZIP compression
  Method 85 (Unzlate) - BPE + ANS (reserved, high overhead)
  Method 86 (Bpelate) - BPE + DEFLATE (source code, text)

The compressor automatically selects the best method for each file.

Examples:
  enz archive document.txt       Compress document.txt into archive.zip
  enz -0 backup.zip data.bin     Store without compression
  enz -v -m docs.zip readme.txt  Compress with verbose output, delete original

`)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "enz: "+format+"\n", args...)
	os.Exit(1)
}
