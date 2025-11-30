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

func TestDecompressRoundtrip(t *testing.T) {
	testCases := []struct {
		name   string
		data   []byte
		method compress.Method
	}{
		{
			name:   "small text unzlate",
			data:   []byte("The quick brown fox jumps over the lazy dog."),
			method: compress.MethodUNZLATE,
		},
		{
			name:   "small text deflate",
			data:   []byte("The quick brown fox jumps over the lazy dog."),
			method: compress.MethodDEFLATE,
		},
		{
			name:   "small text store",
			data:   []byte("The quick brown fox jumps over the lazy dog."),
			method: compress.MethodStore,
		},
		{
			name:   "repetitive deflate",
			data:   bytes.Repeat([]byte("Hello "), 1000),
			method: compress.MethodDEFLATE,
		},
		{
			name:   "binary data",
			data:   makeBinaryData(1024),
			method: compress.MethodDEFLATE,
		},
		{
			name:   "empty file",
			data:   []byte{},
			method: compress.MethodStore,
		},
		{
			name:   "single byte",
			data:   []byte{0x42},
			method: compress.MethodStore,
		},
	}

	comp := compress.New(vocab.Default())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Compress
			compressed, err := comp.CompressFileAs(tc.data, "test.dat", testModTime(), tc.method)
			if err != nil {
				t.Fatalf("compression failed: %v", err)
			}

			// Decompress
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompression failed: %v", err)
			}

			// Verify
			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("roundtrip failed: got %d bytes, want %d bytes", len(decompressed), len(tc.data))
				if len(tc.data) < 100 {
					t.Errorf("got: %q", decompressed)
					t.Errorf("want: %q", tc.data)
				}
			}
		})
	}
}

func TestDecompressAutoMethod(t *testing.T) {
	// Test that auto-detection picks appropriate method
	testCases := []struct {
		name            string
		data            []byte
		expectedMethods []compress.Method // Any of these is acceptable
	}{
		{
			name:            "english text",
			data:            []byte("This is a sample of English text that should be detected as natural language and compressed with BPELATE or DEFLATE."),
			expectedMethods: []compress.Method{compress.MethodBPELATE, compress.MethodDEFLATE},
		},
		{
			name:            "json data",
			data:            []byte(`{"name": "test", "value": 123, "items": ["a", "b", "c"]}`),
			expectedMethods: []compress.Method{compress.MethodDEFLATE, compress.MethodBPELATE},
		},
		{
			name: "source code",
			data: []byte(`func main() {
	fmt.Println("Hello, World!")
	for i := 0; i < 10; i++ {
		fmt.Printf("%d\n", i)
	}
}`),
			expectedMethods: []compress.Method{compress.MethodDEFLATE, compress.MethodBPELATE},
		},
	}

	comp := compress.New(vocab.Default())

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := comp.CompressFile(tc.data, "test.dat", testModTime())
			if err != nil {
				t.Fatalf("compression failed: %v", err)
			}

			info, err := compress.GetFileInfo(compressed)
			if err != nil {
				t.Fatalf("failed to get file info: %v", err)
			}

			// Check if method is one of the expected ones
			methodOK := false
			for _, m := range tc.expectedMethods {
				if info.Method == m {
					methodOK = true
					break
				}
			}
			if !methodOK {
				t.Errorf("method mismatch: got %v, want one of %v", info.Method, tc.expectedMethods)
			}

			// Verify roundtrip (this is the important part)
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompression failed: %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Error("roundtrip verification failed")
			}
		})
	}
}

func TestDecompressInvalidFormat(t *testing.T) {
	comp := compress.New(vocab.Default())

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{0x55, 0x4e, 0x5a}},
		{"wrong magic", []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"random bytes", []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := comp.Decompress(tc.data)
			if err == nil {
				t.Error("expected error for invalid format")
			}
		})
	}
}

func TestDecompressToFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create compressed file
	inputData := []byte("Test data for file decompression")
	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFile(inputData, "output.txt", testModTime())
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	archivePath := filepath.Join(tmpDir, "test.unz")
	if err := os.WriteFile(archivePath, compressed, 0644); err != nil {
		t.Fatalf("failed to write archive: %v", err)
	}

	// Decompress
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "output.txt")
	if err := os.WriteFile(outputPath, decompressed, 0644); err != nil {
		t.Fatalf("failed to write output: %v", err)
	}

	// Verify
	readBack, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	if !bytes.Equal(readBack, inputData) {
		t.Error("file content mismatch")
	}
}

func TestFileInfo(t *testing.T) {
	inputData := []byte("Test data for file info")
	modTime := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFile(inputData, "document.txt", modTime)
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	info, err := compress.GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	// Check all fields
	if info.Name != "document.txt" {
		t.Errorf("name: got %q, want %q", info.Name, "document.txt")
	}

	if info.Size != int64(len(inputData)) {
		t.Errorf("size: got %d, want %d", info.Size, len(inputData))
	}

	if info.CompSize <= 0 {
		t.Errorf("compressed size should be positive: got %d", info.CompSize)
	}

	if info.CRC32 == 0 {
		t.Error("CRC32 should not be zero")
	}

	// ModTime should match (DOS format has 2-second precision)
	timeDiff := info.ModTime.Unix() - modTime.Unix()
	if timeDiff < -2 || timeDiff > 2 {
		t.Errorf("modtime: got %v, want ~%v", info.ModTime, modTime)
	}
}

func TestIsValidFormat(t *testing.T) {
	comp := compress.New(vocab.Default())

	// Valid compressed data
	compressed, _ := comp.CompressFile([]byte("test"), "test.txt", testModTime())
	if !compress.IsValidFormat(compressed) {
		t.Error("should recognize valid format")
	}

	// Invalid data
	if compress.IsValidFormat([]byte{}) {
		t.Error("should reject empty data")
	}

	if compress.IsValidFormat([]byte("not compressed")) {
		t.Error("should reject invalid data")
	}

	if compress.IsValidFormat([]byte{0x50, 0x4b, 0x03, 0x04}) {
		t.Error("should reject ZIP format")
	}
}

func TestLargeFile(t *testing.T) {
	// Test with larger file (1MB)
	inputData := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 20000)

	comp := compress.New(vocab.Default())
	compressed, err := comp.CompressFile(inputData, "large.txt", testModTime())
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}

	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}

	if !bytes.Equal(decompressed, inputData) {
		t.Errorf("large file roundtrip failed: got %d bytes, want %d bytes", len(decompressed), len(inputData))
	}
}

func makeBinaryData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i * 37 % 256)
	}
	return data
}

func testModTime() time.Time {
	return time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
}
