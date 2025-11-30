package compress

import (
	"bytes"
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
		"文件.txt",  // Chinese
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
		{16383},  // max 2-byte varint
		{16384},  // min 3-byte varint
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
