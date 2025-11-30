package compress

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/ha1tch/unz/pkg/bpe"
)

func testVocab() *bpe.Vocabulary {
	// Create a simple test vocabulary
	tokens := make(map[string]int)
	for i := 0; i < 256; i++ {
		tokens[string([]byte{byte(i)})] = i
	}
	// Add some common pairs
	tokens["th"] = 256
	tokens["he"] = 257
	tokens["in"] = 258
	tokens["er"] = 259
	tokens["the"] = 260
	return bpe.NewVocabulary(tokens)
}

func testTime() time.Time {
	return time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
}

func TestCompressDecompressRoundtrip(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short text", []byte("hello")},
		{"longer text", []byte("the quick brown fox jumps over the lazy dog")},
		{"repetitive", bytes.Repeat([]byte("abc"), 100)},
		{"binary", makeBinary(256)},
		{"all bytes", makeAllBytes()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := comp.CompressFile(tc.data, "test.dat", testTime())
			if err != nil {
				t.Fatalf("compress failed: %v", err)
			}

			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompress failed: %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("roundtrip failed: got %d bytes, want %d bytes", len(decompressed), len(tc.data))
			}
		})
	}
}

func TestCompressMethods(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	data := []byte("test data for compression methods")

	methods := []Method{MethodUNZLATE, MethodDEFLATE, MethodStore}

	for _, method := range methods {
		t.Run(method.String(), func(t *testing.T) {
			compressed, err := comp.CompressFileAs(data, "test.dat", testTime(), method)
			if err != nil {
				t.Fatalf("compress failed: %v", err)
			}

			info, err := GetFileInfo(compressed)
			if err != nil {
				t.Fatalf("get info failed: %v", err)
			}

			if info.Method != method {
				t.Errorf("method: got %v, want %v", info.Method, method)
			}

			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompress failed: %v", err)
			}

			if !bytes.Equal(decompressed, data) {
				t.Error("roundtrip failed")
			}
		})
	}
}

func TestGetFileInfo(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	data := []byte("test data")
	modTime := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	compressed, err := comp.CompressFile(data, "document.txt", modTime)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	info, err := GetFileInfo(compressed)
	if err != nil {
		t.Fatalf("get info failed: %v", err)
	}

	if info.Name != "document.txt" {
		t.Errorf("name: got %q, want %q", info.Name, "document.txt")
	}

	if info.Size != int64(len(data)) {
		t.Errorf("size: got %d, want %d", info.Size, len(data))
	}

	if info.CRC32 == 0 {
		t.Error("CRC32 should not be zero")
	}

	// ModTime (DOS format has 2-second precision)
	timeDiff := info.ModTime.Unix() - modTime.Unix()
	if timeDiff < -2 || timeDiff > 2 {
		t.Errorf("modtime: got %v, want ~%v", info.ModTime.Unix(), modTime.Unix())
	}
}

func TestIsValidFormat(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	// Valid
	compressed, _ := comp.CompressFile([]byte("test"), "test.txt", testTime())
	if !IsValidFormat(compressed) {
		t.Error("should accept valid format")
	}

	// Invalid cases
	if IsValidFormat(nil) {
		t.Error("should reject nil")
	}
	if IsValidFormat([]byte{}) {
		t.Error("should reject empty")
	}
	if IsValidFormat([]byte{0x50, 0x4b, 0x03, 0x04}) {
		t.Error("should reject ZIP")
	}
	if IsValidFormat(bytes.Repeat([]byte{0}, 100)) {
		t.Error("should reject zeros")
	}
}

func TestMethodString(t *testing.T) {
	testCases := []struct {
		method Method
		want   string
	}{
		{MethodUNZLATE, "Unzlate"},
		{MethodDEFLATE, "Deflate"},
		{MethodStore, "Stored"},
		{Method(99), "Unknown"},
	}

	for _, tc := range testCases {
		if got := tc.method.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.method, got, tc.want)
		}
	}
}

func TestCompressLargeFile(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	// 1MB of repetitive text
	data := bytes.Repeat([]byte("the quick brown fox "), 50000)

	compressed, err := comp.CompressFile(data, "large.txt", testTime())
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	info, _ := GetFileInfo(compressed)

	// Should compress well
	if info.CompSize >= info.Size/2 {
		t.Errorf("poor compression: %d -> %d", info.Size, info.CompSize)
	}

	// Roundtrip
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Error("roundtrip failed")
	}
}

func TestCompressUnicode(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	data := []byte("Hello 世界! Привет мир! مرحبا بالعالم")

	compressed, err := comp.CompressFile(data, "unicode.txt", testTime())
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Error("unicode roundtrip failed")
	}
}

func TestCompressSpecialNames(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	data := []byte("test")

	names := []string{
		"simple.txt",
		"path/to/file.txt",
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"UPPERCASE.TXT",
		"MixedCase.Txt",
		"файл.txt", // Cyrillic
		"文件.txt",   // Chinese
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			compressed, err := comp.CompressFile(data, name, testTime())
			if err != nil {
				t.Fatalf("compress failed: %v", err)
			}

			info, err := GetFileInfo(compressed)
			if err != nil {
				t.Fatalf("get info failed: %v", err)
			}

			if info.Name != name {
				t.Errorf("name: got %q, want %q", info.Name, name)
			}
		})
	}
}

func TestVarints(t *testing.T) {
	testCases := [][]int{
		{},
		{0},
		{1, 2, 3},
		{127},
		{128},
		{255},
		{256},
		{1000},
		{0, 128, 256, 1000, 10000},
		{16383}, // max 2-byte varint
		{16384}, // min 3-byte varint
	}

	for _, values := range testCases {
		encoded := encodeVarints(values)
		decoded := decodeVarints(encoded)

		if len(decoded) != len(values) {
			t.Errorf("length mismatch: got %d, want %d", len(decoded), len(values))
			continue
		}

		for i := range values {
			if decoded[i] != values[i] {
				t.Errorf("value %d: got %d, want %d", i, decoded[i], values[i])
			}
		}
	}
}

func BenchmarkCompress(b *testing.B) {
	vocab := testVocab()
	comp := New(vocab)
	data := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		comp.CompressFile(data, "test.txt", testTime())
	}
}

func BenchmarkDecompress(b *testing.B) {
	vocab := testVocab()
	comp := New(vocab)
	data := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog "), 1000)
	compressed, _ := comp.CompressFile(data, "test.txt", testTime())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		comp.Decompress(compressed)
	}
}

func makeBinary(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i * 37 % 256)
	}
	return data
}

func makeAllBytes() []byte {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}

// Tests for BPELATE compression method
func TestBPELATERoundtrip(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)

	testCases := []struct {
		name string
		data []byte
	}{
		{"short text", []byte("hello world")},
		{"longer text", []byte("the quick brown fox jumps over the lazy dog again and again")},
		{"repetitive", bytes.Repeat([]byte("abc"), 50)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Compress with BPELATE explicitly
			compressed, err := comp.CompressFileAsWithMode(tc.data, "test.dat", testTime(), 0644, MethodBPELATE)
			if err != nil {
				t.Fatalf("compress failed: %v", err)
			}

			// Verify method
			info, err := GetFileInfo(compressed)
			if err != nil {
				t.Fatalf("GetFileInfo failed: %v", err)
			}
			if info.Method != MethodBPELATE {
				t.Errorf("method: got %v, want BPELATE", info.Method)
			}

			// Decompress
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompress failed: %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("roundtrip failed")
			}
		})
	}
}

// Tests for VocabInfo
func TestVocabInfo(t *testing.T) {
	testCases := []struct {
		name string
		info VocabInfo
	}{
		{"empty", VocabInfo{}},
		{"english go", VocabInfo{NatLang: NatLangEnglish, ProgLang: ProgLangGo}},
		{"spanish python", VocabInfo{NatLang: NatLangSpanish, ProgLang: ProgLangPython}},
		{"all fields", VocabInfo{
			NatLang:  NatLangChinese,
			ProgLang: ProgLangRust,
			DataFmt:  DataFmtJSON,
			Markup:   MarkupHTML,
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			extra := makeVocabInfo(tc.info)

			// Parse it back
			parsed, ok := parseVocabInfo(extra)
			if !ok {
				t.Fatal("parseVocabInfo failed")
			}

			if parsed.NatLang != tc.info.NatLang {
				t.Errorf("NatLang: got %v, want %v", parsed.NatLang, tc.info.NatLang)
			}
			if parsed.ProgLang != tc.info.ProgLang {
				t.Errorf("ProgLang: got %v, want %v", parsed.ProgLang, tc.info.ProgLang)
			}
			if parsed.DataFmt != tc.info.DataFmt {
				t.Errorf("DataFmt: got %v, want %v", parsed.DataFmt, tc.info.DataFmt)
			}
			if parsed.Markup != tc.info.Markup {
				t.Errorf("Markup: got %v, want %v", parsed.Markup, tc.info.Markup)
			}
		})
	}
}

// Test legacy 1-byte VocabInfo parsing
func TestVocabInfoLegacy(t *testing.T) {
	// Create a legacy 1-byte extra field
	legacy := make([]byte, 5)
	legacy[0] = byte(extraVocabInfo & 0xFF)
	legacy[1] = byte(extraVocabInfo >> 8)
	legacy[2] = 1 // size: 1 byte
	legacy[3] = 0
	legacy[4] = LangIDGo

	parsed, ok := parseVocabInfo(legacy)
	if !ok {
		t.Fatal("parseVocabInfo failed for legacy format")
	}

	if parsed.ProgLang != ProgLangGo {
		t.Errorf("ProgLang: got %v, want Go", parsed.ProgLang)
	}
}

// Test type String methods
func TestTypeStrings(t *testing.T) {
	// Method String
	methods := []struct {
		m    Method
		want string
	}{
		{MethodStore, "Stored"},
		{MethodDEFLATE, "Deflate"},
		{MethodUNZLATE, "Unzlate"},
		{MethodBPELATE, "Bpelate"},
		{Method(255), "Unknown"},
	}
	for _, tc := range methods {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("Method(%d).String(): got %q, want %q", tc.m, got, tc.want)
		}
	}

	// NatLang String
	natLangs := []struct {
		n    NatLang
		want string
	}{
		{NatLangUnspecified, "unspecified"},
		{NatLangEnglish, "en"},
		{NatLangChinese, "zh"},
		{NatLang(99), "unknown"},
	}
	for _, tc := range natLangs {
		if got := tc.n.String(); got != tc.want {
			t.Errorf("NatLang(%d).String(): got %q, want %q", tc.n, got, tc.want)
		}
	}

	// ProgLang String
	progLangs := []struct {
		p    ProgLang
		want string
	}{
		{ProgLangNone, "none"},
		{ProgLangGo, "go"},
		{ProgLangRust, "rust"},
		{ProgLang(99), "unknown"},
	}
	for _, tc := range progLangs {
		if got := tc.p.String(); got != tc.want {
			t.Errorf("ProgLang(%d).String(): got %q, want %q", tc.p, got, tc.want)
		}
	}

	// DataFmt String
	dataFmts := []struct {
		d    DataFmt
		want string
	}{
		{DataFmtNone, "none"},
		{DataFmtJSON, "json"},
		{DataFmt(99), "unknown"},
	}
	for _, tc := range dataFmts {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("DataFmt(%d).String(): got %q, want %q", tc.d, got, tc.want)
		}
	}

	// MarkupLang String
	markups := []struct {
		m    MarkupLang
		want string
	}{
		{MarkupNone, "none"},
		{MarkupHTML, "html"},
		{MarkupLang(99), "unknown"},
	}
	for _, tc := range markups {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("MarkupLang(%d).String(): got %q, want %q", tc.m, got, tc.want)
		}
	}
}

// === Multi-file Archive Tests ===

func TestArchiveMultipleFiles(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	// Add multiple files
	files := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Hello, World!"},
		{"file2.txt", "Another file with some content."},
		{"src/main.go", "package main\n\nfunc main() {}\n"},
	}

	for _, f := range files {
		err := archive.Add([]byte(f.content), f.name, time.Now(), 0644)
		if err != nil {
			t.Fatalf("Add(%s): %v", f.name, err)
		}
	}

	// Build archive
	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	// Verify it's a valid ZIP
	if !IsValidFormat(data) {
		t.Fatal("Archive is not valid ZIP format")
	}

	// List files
	infos, err := ListFiles(data)
	if err != nil {
		t.Fatalf("ListFiles(): %v", err)
	}

	if len(infos) != len(files) {
		t.Errorf("ListFiles: got %d files, want %d", len(infos), len(files))
	}

	// Verify each file
	for i, f := range files {
		if infos[i].Name != f.name {
			t.Errorf("File %d: name = %q, want %q", i, infos[i].Name, f.name)
		}
		if infos[i].Size != int64(len(f.content)) {
			t.Errorf("File %d: size = %d, want %d", i, infos[i].Size, len(f.content))
		}
	}

	// Extract each file
	for i, f := range files {
		content, err := comp.DecompressFile(data, infos[i])
		if err != nil {
			t.Errorf("DecompressFile(%s): %v", f.name, err)
			continue
		}
		if string(content) != f.content {
			t.Errorf("DecompressFile(%s): content = %q, want %q", f.name, string(content), f.content)
		}
	}
}

func TestArchiveWithDirectories(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	// Add directory
	err := archive.AddDirectory("mydir", time.Now(), 0755)
	if err != nil {
		t.Fatalf("AddDirectory: %v", err)
	}

	// Add file in directory
	err = archive.Add([]byte("file content"), "mydir/file.txt", time.Now(), 0644)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Build archive
	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	// List files
	infos, err := ListFiles(data)
	if err != nil {
		t.Fatalf("ListFiles(): %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("ListFiles: got %d entries, want 2", len(infos))
	}

	// First should be directory
	if infos[0].Name != "mydir/" {
		t.Errorf("Directory name = %q, want %q", infos[0].Name, "mydir/")
	}
	if infos[0].Size != 0 {
		t.Errorf("Directory size = %d, want 0", infos[0].Size)
	}

	// Second should be file
	if infos[1].Name != "mydir/file.txt" {
		t.Errorf("File name = %q, want %q", infos[1].Name, "mydir/file.txt")
	}
}

func TestArchiveStore(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	content := []byte("This content should be stored without compression")

	err := archive.AddStore(content, "stored.bin", time.Now(), 0644)
	if err != nil {
		t.Fatalf("AddStore: %v", err)
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	infos, err := ListFiles(data)
	if err != nil {
		t.Fatalf("ListFiles(): %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("ListFiles: got %d files, want 1", len(infos))
	}

	// Method should be Stored
	if infos[0].Method != MethodStore {
		t.Errorf("Method = %v, want Stored", infos[0].Method)
	}

	// Compressed size should equal original size
	if infos[0].CompSize != infos[0].Size {
		t.Errorf("CompSize = %d, Size = %d, want equal", infos[0].CompSize, infos[0].Size)
	}

	// Extract and verify
	extracted, err := comp.DecompressFile(data, infos[0])
	if err != nil {
		t.Fatalf("DecompressFile: %v", err)
	}
	if !bytes.Equal(extracted, content) {
		t.Errorf("Extracted content doesn't match original")
	}
}

func TestDecompressAll(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	files := map[string]string{
		"a.txt":     "Content of A",
		"b.txt":     "Content of B",
		"dir/c.txt": "Content of C",
	}

	// Add directory first
	archive.AddDirectory("dir", time.Now(), 0755)

	for name, content := range files {
		err := archive.Add([]byte(content), name, time.Now(), 0644)
		if err != nil {
			t.Fatalf("Add(%s): %v", name, err)
		}
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	// DecompressAll
	extracted, err := comp.DecompressAll(data)
	if err != nil {
		t.Fatalf("DecompressAll(): %v", err)
	}

	// Should have 3 files (directory not included)
	if len(extracted) != len(files) {
		t.Errorf("DecompressAll: got %d files, want %d", len(extracted), len(files))
	}

	// Verify each file
	for name, want := range files {
		got, ok := extracted[name]
		if !ok {
			t.Errorf("DecompressAll: missing file %q", name)
			continue
		}
		if string(got) != want {
			t.Errorf("DecompressAll(%s): got %q, want %q", name, string(got), want)
		}
	}
}

func TestListFilesEmpty(t *testing.T) {
	// Test with invalid/empty data
	_, err := ListFiles(nil)
	if err != ErrTooShort {
		t.Errorf("ListFiles(nil): got %v, want ErrTooShort", err)
	}

	_, err = ListFiles([]byte{1, 2, 3})
	if err != ErrTooShort {
		t.Errorf("ListFiles(short): got %v, want ErrTooShort", err)
	}
}

func TestArchiveEmpty(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	// Build empty archive
	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	// Should still be valid ZIP
	if !IsValidFormat(data) {
		// Empty archive might not have local file header
		// but should at least have EOCD
		if len(data) < 22 {
			t.Error("Empty archive too short")
		}
	}

	// ListFiles should return empty
	infos, err := ListFiles(data)
	if err != nil && len(data) >= 22 {
		// Only error if we have enough data for EOCD
		t.Logf("ListFiles on empty archive: %v (may be expected)", err)
	}
	if len(infos) != 0 {
		t.Errorf("ListFiles on empty: got %d files, want 0", len(infos))
	}
}

func TestArchiveMixedMethods(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	// Add files that will use different compression methods
	files := []struct {
		name    string
		content []byte
		store   bool
	}{
		{"code.go", []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), false},
		{"binary.bin", []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}, true},
		{"text.txt", []byte("The quick brown fox jumps over the lazy dog."), false},
	}

	for _, f := range files {
		var err error
		if f.store {
			err = archive.AddStore(f.content, f.name, time.Now(), 0644)
		} else {
			err = archive.Add(f.content, f.name, time.Now(), 0644)
		}
		if err != nil {
			t.Fatalf("Add(%s): %v", f.name, err)
		}
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	// Extract all and verify
	extracted, err := comp.DecompressAll(data)
	if err != nil {
		t.Fatalf("DecompressAll(): %v", err)
	}

	for _, f := range files {
		got, ok := extracted[f.name]
		if !ok {
			t.Errorf("Missing file: %s", f.name)
			continue
		}
		if !bytes.Equal(got, f.content) {
			t.Errorf("Content mismatch for %s", f.name)
		}
	}
}

func TestArchivePreservesMetadata(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	modTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	mode := os.FileMode(0755)

	err := archive.Add([]byte("test content"), "test.sh", modTime, mode)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	infos, err := ListFiles(data)
	if err != nil {
		t.Fatalf("ListFiles(): %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(infos))
	}

	info := infos[0]

	// Check mode (upper bits)
	if info.Mode&0777 != mode&0777 {
		t.Errorf("Mode: got %o, want %o", info.Mode&0777, mode&0777)
	}

	// Check modification time (DOS time has 2-second resolution)
	timeDiff := info.ModTime.Sub(modTime)
	if timeDiff < -2*time.Second || timeDiff > 2*time.Second {
		t.Errorf("ModTime: got %v, want ~%v", info.ModTime, modTime)
	}
}

func TestDecompressFileByOffset(t *testing.T) {
	vocab := testVocab()
	comp := New(vocab)
	archive := NewArchive(comp)

	// Add files
	archive.Add([]byte("first file"), "first.txt", time.Now(), 0644)
	archive.Add([]byte("second file"), "second.txt", time.Now(), 0644)
	archive.Add([]byte("third file"), "third.txt", time.Now(), 0644)

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes(): %v", err)
	}

	infos, err := ListFiles(data)
	if err != nil {
		t.Fatalf("ListFiles(): %v", err)
	}

	// Extract second file specifically
	content, err := comp.DecompressFile(data, infos[1])
	if err != nil {
		t.Fatalf("DecompressFile: %v", err)
	}

	if string(content) != "second file" {
		t.Errorf("Got %q, want %q", string(content), "second file")
	}
}
