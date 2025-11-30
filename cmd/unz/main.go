// Command unz decompresses ZIP files.
//
// Usage matches unzip(1):
//
//	unz [-ltvqonp] [-d dir] archive.zip [file...]
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
		fatal("'%s' is not a valid ZIP archive", archivePath)
	}

	// Get all files in archive
	files, err := compress.ListFiles(data)
	if err != nil {
		fatal("cannot read archive: %v", err)
	}

	// Collect patterns to extract (if specified)
	patterns := flag.Args()[1:]

	// List mode
	if *list || *listVerbose {
		printListing(archivePath, files, *listVerbose)
		return
	}

	// Test mode
	if *test {
		testArchive(archivePath, data, files)
		return
	}

	// Extract
	extractFiles(archivePath, data, files, patterns)
}

func printListing(archivePath string, files []*compress.FileInfo, verbose bool) {
	fmt.Printf("Archive:  %s\n", archivePath)

	var totalSize, totalComp int64

	if verbose {
		// Verbose format matching unzip -v
		fmt.Println(" Length   Method     Size  Cmpr    Date    Time   CRC-32   Name")
		fmt.Println("--------  ------  -------- ---- ---------- ----- --------  ----")

		for _, info := range files {
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

			totalSize += info.Size
			totalComp += info.CompSize
		}

		totalRatio := 0
		if totalSize > 0 {
			totalRatio = 100 - int(totalComp*100/totalSize)
		}
		fmt.Println("--------          -------- ----                            -------")
		fmt.Printf("%8d          %8d %3d%%                            %d file%s\n",
			totalSize, totalComp, totalRatio, len(files), plural(len(files)))
	} else {
		// Short format matching unzip -l
		fmt.Println("  Length      Date    Time    Name")
		fmt.Println("---------  ---------- -----   ----")

		for _, info := range files {
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

			totalSize += info.Size
		}

		fmt.Println("---------                     -------")
		fmt.Printf("%9d                     %d file%s\n", totalSize, len(files), plural(len(files)))
	}
}

func testArchive(archivePath string, data []byte, files []*compress.FileInfo) {
	vocab := vocab.Default()
	decomp := compress.New(vocab)

	errors := 0
	for _, info := range files {
		// Skip directories
		if strings.HasSuffix(info.Name, "/") {
			continue
		}

		if !*quiet {
			fmt.Printf("    testing: %-40s ", info.Name)
		}

		_, err := decomp.DecompressFile(data, info)
		if err != nil {
			if !*quiet {
				fmt.Println("error")
			}
			fmt.Fprintf(os.Stderr, "  %s: %v\n", info.Name, err)
			errors++
		} else {
			if !*quiet {
				fmt.Println("OK")
			}
		}
	}

	if errors > 0 {
		fmt.Fprintf(os.Stderr, "%d error(s) detected in %s\n", errors, archivePath)
		os.Exit(1)
	}
	fmt.Println("No errors detected in compressed data of", archivePath)
}

func extractFiles(archivePath string, data []byte, files []*compress.FileInfo, patterns []string) {
	vocab := vocab.Default()
	decomp := compress.New(vocab)

	for _, info := range files {
		// Check if file matches patterns (if any)
		if len(patterns) > 0 && !matchesAny(info.Name, patterns) {
			continue
		}

		// Determine output path
		outputPath := info.Name
		if *junkPaths {
			outputPath = filepath.Base(outputPath)
		}
		if *destDir != "" {
			outputPath = filepath.Join(*destDir, outputPath)
		}

		// Handle directory
		if strings.HasSuffix(info.Name, "/") {
			if !*pipe {
				if !*quiet {
					fmt.Printf("   creating: %s\n", outputPath)
				}
				os.MkdirAll(outputPath, info.Mode|0755)
			}
			continue
		}

		// Check if output exists
		if !*overwrite && !*pipe {
			if _, err := os.Stat(outputPath); err == nil {
				if *never {
					if !*quiet {
						fmt.Printf("  skipping: %s\n", outputPath)
					}
					continue
				}
				// Prompt (like unzip)
				fmt.Printf("replace %s? [y]es, [n]o, [A]ll, [N]one: ", outputPath)
				var response string
				fmt.Scanln(&response)
				response = strings.ToLower(response)
				if response == "a" || response == "all" {
					*overwrite = true
				} else if response == "none" {
					*never = true
					fmt.Println("  skipping:", outputPath)
					continue
				} else if response != "y" && response != "yes" {
					fmt.Println("  skipping:", outputPath)
					continue
				}
			}
		}

		// Decompress
		if !*quiet && !*pipe {
			fmt.Printf("  inflating: %s\n", outputPath)
		}

		content, err := decomp.DecompressFile(data, info)
		if err != nil {
			fatal("decompression failed for '%s': %v", info.Name, err)
		}

		// Write output
		if *pipe {
			os.Stdout.Write(content)
		} else {
			// Create parent directories if needed
			if dir := filepath.Dir(outputPath); dir != "." {
				os.MkdirAll(dir, 0755)
			}

			if err := os.WriteFile(outputPath, content, info.Mode); err != nil {
				fatal("cannot write '%s': %v", outputPath, err)
			}

			// Set modification time if available
			if !info.ModTime.IsZero() {
				os.Chtimes(outputPath, info.ModTime, info.ModTime)
			}
		}
	}
}

// matchesAny checks if name matches any of the patterns.
func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
		// Also try matching against base name
		matched, err = filepath.Match(pattern, filepath.Base(name))
		if err == nil && matched {
			return true
		}
	}
	return false
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
  unz archive.zip '*.txt'          Extract only .txt files
  unz -p archive.zip > file        Extract to stdout

`)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "unz: "+format+"\n", args...)
	os.Exit(1)
}
