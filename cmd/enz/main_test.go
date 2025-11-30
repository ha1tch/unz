package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ha1tch/unz/pkg/compress"
	"github.com/ha1tch/unz/pkg/vocab"
)

func TestCompressFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create test input file
	inputPath := filepath.Join(tmpDir, "test.txt")
	inputData := []byte("The quick brown fox jumps over the lazy dog. This is a test of compression.")
	if err := os.WriteFile(inputPath, inputData, 0644); err != nil {
		t.Fatalf("failed to create input file: %v", err)
	}

	// Compress
	outputPath := filepath.Join(tmpDir, "test.unz")
	comp := compress.New(vocab.Default())
	info, _ := os.Stat(inputPath)

	compressed, err := comp.CompressFile(inputData, "test.txt", info.ModTime())
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	if err := os.WriteFile(outputPath, compressed, 0644); err != nil {
		t.Fatalf("failed to write output: %v", err)
	}

	// Verify output exists and is valid
	if !compress.IsValidFormat(compressed) {
		t.Error("output is not valid UNZ format")
	}

	// Verify file info
	fileInfo, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	if fileInfo.Name != "test.txt" {
		t.Errorf("name mismatch: got %q, want %q", fileInfo.Name, "test.txt")
	}

	if fileInfo.Size != int64(len(inputData)) {
		t.Errorf("size mismatch: got %d, want %d", fileInfo.Size, len(inputData))
	}
}

func TestCompressStore(t *testing.T) {
	// Test store method (no compression)
	inputData := []byte("Short test data")

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFileAs(inputData, "test.txt", testModTime(), compress.MethodStore)
	if err != nil {
		t.Fatalf("store failed: %v", err)
	}

	info, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	if info.Method != compress.MethodStore {
		t.Errorf("method mismatch: got %v, want %v", info.Method, compress.MethodStore)
	}

	// Stored data should be same size as original (plus header)
	if info.CompSize != int64(len(inputData)) {
		t.Errorf("stored size mismatch: got %d, want %d", info.CompSize, len(inputData))
	}
}

func TestCompressDeflate(t *testing.T) {
	// Test DEFLATE method
	inputData := bytes.Repeat([]byte("Hello World! "), 100)

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFileAs(inputData, "test.txt", testModTime(), compress.MethodDEFLATE)
	if err != nil {
		t.Fatalf("deflate failed: %v", err)
	}

	info, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	if info.Method != compress.MethodDEFLATE {
		t.Errorf("method mismatch: got %v, want %v", info.Method, compress.MethodDEFLATE)
	}

	// DEFLATE should compress repetitive data well
	if info.CompSize >= info.Size {
		t.Errorf("DEFLATE should compress repetitive data: got %d >= %d", info.CompSize, info.Size)
	}
}

func TestCompressUnzlate(t *testing.T) {
	// Test UNZLATE method
	inputData := []byte("The quick brown fox jumps over the lazy dog. " +
		"This sentence contains common English words that should compress well " +
		"with the BPE vocabulary trained on English text.")

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFileAs(inputData, "test.txt", testModTime(), compress.MethodUNZLATE)
	if err != nil {
		t.Fatalf("unzlate failed: %v", err)
	}

	info, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	if info.Method != compress.MethodUNZLATE {
		t.Errorf("method mismatch: got %v, want %v", info.Method, compress.MethodUNZLATE)
	}
}

func TestCompressEmptyFile(t *testing.T) {
	inputData := []byte{}

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFile(inputData, "empty.txt", testModTime())
	if err != nil {
		t.Fatalf("compression of empty file failed: %v", err)
	}

	info, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	if info.Size != 0 {
		t.Errorf("size should be 0, got %d", info.Size)
	}
}

func TestCompressJunkPaths(t *testing.T) {
	inputData := []byte("test data")

	comp := compress.New(vocab.Default())

	// With full path
	compressed1, _ := comp.CompressFile(inputData, "path/to/file.txt", testModTime())
	info1, _ := compress.GetFileInfo(compressed1)

	if info1.Name != "path/to/file.txt" {
		t.Errorf("expected full path, got %q", info1.Name)
	}

	// With junked path (just filename)
	compressed2, _ := comp.CompressFile(inputData, "file.txt", testModTime())
	info2, _ := compress.GetFileInfo(compressed2)

	if info2.Name != "file.txt" {
		t.Errorf("expected filename only, got %q", info2.Name)
	}
}

func TestCompressCRC32(t *testing.T) {
	inputData := []byte("Test data for CRC32 verification")

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFile(inputData, "test.txt", testModTime())
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	info, _ := compress.GetFileInfo(compressed)

	// CRC32 should be non-zero for non-empty data
	if info.CRC32 == 0 {
		t.Error("CRC32 should not be zero")
	}

	// Compress same data again - CRC should match
	compressed2, _ := comp.CompressFile(inputData, "test.txt", testModTime())
	info2, _ := compress.GetFileInfo(compressed2)

	if info.CRC32 != info2.CRC32 {
		t.Errorf("CRC32 mismatch for same data: %08x vs %08x", info.CRC32, info2.CRC32)
	}

	// Different data should have different CRC
	compressed3, _ := comp.CompressFile([]byte("Different data"), "test.txt", testModTime())
	info3, _ := compress.GetFileInfo(compressed3)

	if info.CRC32 == info3.CRC32 {
		t.Error("CRC32 should differ for different data")
	}
}

func testModTime() time.Time {
	return time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
}
