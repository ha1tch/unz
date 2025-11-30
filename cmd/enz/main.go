// Command enz compresses files using adaptive BPE/DEFLATE compression.
//
// Usage matches zip(1):
//
//	enz [-0|-9] [-r] [-q] [-v] [-m] [-j] archive.zip file...
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
	recursive = flag.Bool("r", false, "recurse into directories")
	quiet     = flag.Bool("q", false, "quiet operation")
	verbose   = flag.Bool("v", false, "verbose operation")
	move      = flag.Bool("m", false, "move into archive (delete input files)")
	junkPaths = flag.Bool("j", false, "junk (don't record) directory names")
	help      = flag.Bool("h", false, "display this help")
)

type fileEntry struct {
	path  string // path on disk
	name  string // name in archive
	info  os.FileInfo
	isDir bool
}

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

	// Collect all files to add
	var entries []fileEntry
	for i := 1; i < flag.NArg(); i++ {
		inputPath := flag.Arg(i)
		collected, err := collectFiles(inputPath)
		if err != nil {
			fatal("cannot access '%s': %v", inputPath, err)
		}
		entries = append(entries, collected...)
	}

	if len(entries) == 0 {
		fatal("no files to add")
	}

	// Create archive
	comp := compress.New(vocab.Default())
	archive := compress.NewArchive(comp)

	var totalIn, totalOut int64
	start := time.Now()

	for _, entry := range entries {
		if entry.isDir {
			// Add directory entry
			if !*quiet {
				fmt.Fprintf(os.Stderr, "  adding: %s/\n", entry.name)
			}
			archive.AddDirectory(entry.name, entry.info.ModTime(), entry.info.Mode())
			continue
		}

		// Read file
		data, err := os.ReadFile(entry.path)
		if err != nil {
			fatal("cannot read '%s': %v", entry.path, err)
		}

		if !*quiet {
			fmt.Fprintf(os.Stderr, "  adding: %s", entry.name)
		}

		// Add to archive
		mode := entry.info.Mode()
		if *level0 {
			err = archive.AddStore(data, entry.name, entry.info.ModTime(), mode)
		} else {
			err = archive.Add(data, entry.name, entry.info.ModTime(), mode)
		}

		if err != nil {
			fatal("compression failed for '%s': %v", entry.path, err)
		}

		totalIn += int64(len(data))

		if !*quiet {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}

	// Write archive
	output, err := archive.Bytes()
	if err != nil {
		fatal("cannot create archive: %v", err)
	}
	totalOut = int64(len(output))

	if err := os.WriteFile(archivePath, output, 0644); err != nil {
		fatal("cannot write '%s': %v", archivePath, err)
	}

	elapsed := time.Since(start)

	// Summary
	if *verbose {
		ratio := float64(0)
		if totalIn > 0 {
			ratio = 100 - (float64(totalOut) * 100 / float64(totalIn))
		}
		fmt.Fprintf(os.Stderr, "total %d bytes -> %d bytes (%.1f%%) in %v\n",
			totalIn, totalOut, ratio, elapsed.Round(time.Millisecond))
	}

	// Delete inputs if -m
	if *move {
		for _, entry := range entries {
			if !entry.isDir {
				os.Remove(entry.path)
			}
		}
		// Remove directories in reverse order (deepest first)
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].isDir {
				os.Remove(entries[i].path)
			}
		}
	}
}

// collectFiles collects files from a path, recursing into directories if -r is set.
func collectFiles(path string) ([]fileEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var entries []fileEntry

	if info.IsDir() {
		if !*recursive {
			return nil, fmt.Errorf("'%s' is a directory (use -r to recurse)", path)
		}

		// Walk directory
		err := filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Get archive name
			name := p
			if *junkPaths {
				name = filepath.Base(p)
			}
			// Normalize path separators
			name = filepath.ToSlash(name)

			entries = append(entries, fileEntry{
				path:  p,
				name:  name,
				info:  fi,
				isDir: fi.IsDir(),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		// Single file
		name := path
		if *junkPaths {
			name = filepath.Base(path)
		}
		name = filepath.ToSlash(name)

		entries = append(entries, fileEntry{
			path:  path,
			name:  name,
			info:  info,
			isDir: false,
		})
	}

	return entries, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: enz [-0|-9] [-r] [-qvmj] archive[.zip] file...

Compress files into ZIP archive using adaptive BPE/DEFLATE compression.
Output is standard PKZIP format compatible with unzip, WinZip, etc.

Options:
  -0        store only (no compression)
  -9        best compression (default)
  -r        recurse into directories
  -q        quiet operation
  -v        verbose operation  
  -m        move into archive (delete input files after compression)
  -j        junk directory names (store only file names)
  -h        display this help

Compression methods:
  Method 0  (Stored)  - no compression
  Method 8  (Deflate) - standard ZIP compression
  Method 85 (Unzlate) - BPE + ANS (reserved, high overhead)
  Method 86 (Bpelate) - BPE + DEFLATE (source code, text)

The compressor automatically selects the best method for each file.

Examples:
  enz archive.zip file.txt          Compress single file
  enz archive.zip *.go              Compress multiple files
  enz -r project.zip src/           Compress directory recursively
  enz -0 backup.zip data.bin        Store without compression
  enz -v -m docs.zip readme.txt     Verbose, delete original after

`)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "enz: "+format+"\n", args...)
	os.Exit(1)
}
