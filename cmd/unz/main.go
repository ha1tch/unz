// Command unz decompresses .unz files.
//
// Usage matches unzip(1):
//
//	unz [-ltvqonp] [-d dir] archive.unz [file...]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ha1tch/unz/pkg/compress"
	"github.com/ha1tch/unz/pkg/vocab"
)

var (
	list        = flag.Bool("l", false, "list files (short format)")
	listVerbose = flag.Bool("v", false, "list files (verbose format)")
	test        = flag.Bool("t", false, "test archive integrity")
	quiet       = flag.Bool("q", false, "quiet operation")
	overwrite   = flag.Bool("o", false, "overwrite files without prompting")
	never       = flag.Bool("n", false, "never overwrite existing files")
	pipe        = flag.Bool("p", false, "extract to stdout (pipe)")
	junkPaths   = flag.Bool("j", false, "junk paths (extract to current directory)")
	destDir     = flag.String("d", "", "extract files into dir")
	help        = flag.Bool("h", false, "display this help")
)

func main() {
	flag.Usage = usage
	flag.Parse()

	if *help {
		usage()
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "unz: missing archive argument")
		fmt.Fprintln(os.Stderr, "Try 'unz -h' for more information.")
		os.Exit(1)
	}

	archivePath := flag.Arg(0)

	// Read archive
	data, err := os.ReadFile(archivePath)
	if err != nil {
		fatal("cannot open '%s': %v", archivePath, err)
	}

	// Validate format
	if !compress.IsValidFormat(data) {
		fatal("'%s' is not a valid .unz archive", archivePath)
	}

	// Get file info
	info, err := compress.GetFileInfo(data)
	if err != nil {
		fatal("cannot read archive: %v", err)
	}

	// List mode
	if *list || *listVerbose {
		printListing(archivePath, info, *listVerbose)
		return
	}

	// Test mode
	if *test {
		testArchive(archivePath, data, info)
		return
	}

	// Extract
	extractFile(archivePath, data, info)
}

func printListing(archivePath string, info *compress.FileInfo, verbose bool) {
	fmt.Printf("Archive:  %s\n", archivePath)

	if verbose {
		// Verbose format matching unzip -v
		fmt.Println(" Length   Method     Size  Cmpr    Date    Time   CRC-32   Name")
		fmt.Println("--------  ------  -------- ---- ---------- ----- --------  ----")

		comprRatio := 0
		if info.Size > 0 {
			comprRatio = 100 - int(info.CompSize*100/info.Size)
			if comprRatio < 0 {
				comprRatio = 0
			}
		}

		dateStr := info.ModTime.Format("2006-01-02")
		timeStr := info.ModTime.Format("15:04")
		if info.ModTime.IsZero() {
			dateStr = "----------"
			timeStr = "-----"
		}

		fmt.Printf("%8d  %-6s  %8d %3d%% %s %s %08x  %s\n",
			info.Size,
			info.Method.String(),
			info.CompSize,
			comprRatio,
			dateStr,
			timeStr,
			info.CRC32,
			info.Name)

		fmt.Println("--------          -------- ----                            -------")
		fmt.Printf("%8d          %8d %3d%%                            1 file\n",
			info.Size, info.CompSize, comprRatio)
	} else {
		// Short format matching unzip -l
		fmt.Println("  Length      Date    Time    Name")
		fmt.Println("---------  ---------- -----   ----")

		dateStr := info.ModTime.Format("2006-01-02")
		timeStr := info.ModTime.Format("15:04")
		if info.ModTime.IsZero() {
			dateStr = "----------"
			timeStr = "-----"
		}

		fmt.Printf("%9d  %s %s   %s\n",
			info.Size,
			dateStr,
			timeStr,
			info.Name)

		fmt.Println("---------                     -------")
		fmt.Printf("%9d                     1 file\n", info.Size)
	}
}

func testArchive(archivePath string, data []byte, info *compress.FileInfo) {
	if !*quiet {
		fmt.Printf("    testing: %-40s ", info.Name)
	}

	// Create decompressor
	vocab := vocab.Default()
	decomp := compress.New(vocab)

	// Decompress
	_, err := decomp.Decompress(data)
	if err != nil {
		if !*quiet {
			fmt.Println("error")
		}
		fatal("test failed: %v", err)
	}

	if !*quiet {
		fmt.Println("OK")
	}

	fmt.Println("No errors detected in compressed data of", archivePath)
}

func extractFile(archivePath string, data []byte, info *compress.FileInfo) {
	// Determine output path
	outputPath := info.Name
	if outputPath == "" {
		// No name stored - use archive name without extension
		outputPath = filepath.Base(archivePath)
		outputPath = strings.TrimSuffix(outputPath, ".zip")
		outputPath = strings.TrimSuffix(outputPath, ".unz")
	}

	if *junkPaths {
		outputPath = filepath.Base(outputPath)
	}

	if *destDir != "" {
		outputPath = filepath.Join(*destDir, outputPath)
	}

	// Check if output exists
	if !*overwrite && !*pipe {
		if _, err := os.Stat(outputPath); err == nil {
			if *never {
				if !*quiet {
					fmt.Printf("  skipping: %s\n", outputPath)
				}
				return
			}
			// Prompt (like unzip)
			fmt.Printf("replace %s? [y]es, [n]o, [A]ll, [N]one: ", outputPath)
			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(response)
			if response != "y" && response != "yes" && response != "a" && response != "all" {
				fmt.Println("  skipping:", outputPath)
				return
			}
		}
	}

	// Create decompressor
	vocab := vocab.Default()
	decomp := compress.New(vocab)

	// Decompress
	if !*quiet && !*pipe {
		fmt.Printf("  inflating: %s\n", outputPath)
	}

	output, err := decomp.Decompress(data)
	if err != nil {
		fatal("decompression failed: %v", err)
	}

	// Write output
	if *pipe {
		os.Stdout.Write(output)
	} else {
		// Create parent directories if needed
		if dir := filepath.Dir(outputPath); dir != "." {
			os.MkdirAll(dir, 0755)
		}

		if err := os.WriteFile(outputPath, output, 0644); err != nil {
			fatal("cannot write '%s': %v", outputPath, err)
		}

		// Set modification time if available
		if !info.ModTime.IsZero() {
			os.Chtimes(outputPath, info.ModTime, info.ModTime)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: unz [-ltvqonpj] [-d dir] archive[.zip] [file...]

Extract files from ZIP archive. Supports standard ZIP plus BPE methods.

Options:
  -l        list files (short format)
  -v        list files with verbose information
  -t        test archive integrity
  -q        quiet operation
  -o        overwrite files without prompting
  -n        never overwrite existing files
  -p        extract to stdout (pipe)
  -j        junk paths (extract to current directory)
  -d dir    extract files into specified directory
  -h        display this help

Supported methods:
  Method 0  (Stored)  - no compression
  Method 8  (Deflate) - standard ZIP compression
  Method 85 (Unzlate) - BPE + ANS
  Method 86 (Bpelate) - BPE + DEFLATE

Examples:
  unz archive.zip                  Extract all files
  unz -l archive.zip               List contents
  unz -v archive.zip               List with details (method, CRC, etc.)
  unz -t archive.zip               Test archive integrity
  unz -d /tmp archive.zip          Extract to /tmp
  unz -p archive.zip > file        Extract to stdout

`)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "unz: "+format+"\n", args...)
	os.Exit(1)
}
