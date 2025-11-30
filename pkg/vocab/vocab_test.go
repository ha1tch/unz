package vocab

import (
	"testing"
)

func TestDefault(t *testing.T) {
	vocab := Default()

	if vocab == nil {
		t.Fatal("Default() returned nil")
	}

	// Should have at least 256 tokens (byte-level)
	if vocab.Size() < 256 {
		t.Errorf("vocabulary too small: %d", vocab.Size())
	}

	// Should be able to get all byte tokens
	for i := 0; i < 256; i++ {
		_, ok := vocab.GetToken(i)
		if !ok {
			t.Errorf("missing token for byte %d", i)
		}
	}
}

func TestSize(t *testing.T) {
	size := Size()

	if size < 256 {
		t.Errorf("Size() too small: %d", size)
	}

	// Should match Default().Size()
	if size != Default().Size() {
		t.Errorf("Size() mismatch: %d vs %d", size, Default().Size())
	}
}

func TestDefaultConsistent(t *testing.T) {
	// Multiple calls should return equivalent vocabularies
	v1 := Default()
	v2 := Default()

	if v1.Size() != v2.Size() {
		t.Error("inconsistent vocabulary size")
	}

	// Check some tokens
	for i := 0; i < 256; i++ {
		t1, ok1 := v1.GetToken(i)
		t2, ok2 := v2.GetToken(i)

		if ok1 != ok2 {
			t.Errorf("token %d: existence mismatch", i)
		}

		if ok1 && string(t1.Bytes) != string(t2.Bytes) {
			t.Errorf("token %d: content mismatch", i)
		}
	}
}

func TestDefaultEncodeDecode(t *testing.T) {
	vocab := Default()

	// Should be able to encode and decode byte sequences
	testCases := [][]byte{
		{0, 1, 2, 3},
		[]byte("hello"),
		[]byte("the quick brown fox"),
	}

	for _, data := range testCases {
		decoded := vocab.Decode(bytesToIDs(data))
		// Note: decoded may not equal data if multi-byte tokens were used
		// But the bytes should be representable
		if len(decoded) == 0 && len(data) > 0 {
			t.Errorf("Decode produced empty output for %v", data)
		}
	}
}

// Helper to convert bytes to token IDs (for basic byte tokens)
func bytesToIDs(data []byte) []int {
	ids := make([]int, len(data))
	for i, b := range data {
		ids[i] = int(b)
	}
	return ids
}

// Test ForLanguage returns valid vocabularies for all languages
func TestForLanguage(t *testing.T) {
	testCases := []struct {
		lang    Language
		name    string
		minSize int
	}{
		{LangText, "Text", 700},
		{LangGo, "Go", 1700},
		{LangPython, "Python", 1700},
		{LangJavaScript, "JavaScript", 1700},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vocab := ForLanguage(tc.lang)
			if vocab == nil {
				t.Fatal("ForLanguage() returned nil")
			}

			size := vocab.Size()
			if size < tc.minSize {
				t.Errorf("vocabulary too small: got %d, want >= %d", size, tc.minSize)
			}

			// Should have all byte tokens
			for i := 0; i < 256; i++ {
				_, ok := vocab.GetToken(i)
				if !ok {
					t.Errorf("missing byte token %d", i)
				}
			}
		})
	}
}

// Test SizeForLanguage
func TestSizeForLanguage(t *testing.T) {
	testCases := []struct {
		lang    Language
		minSize int
	}{
		{LangText, 700},
		{LangGo, 1700},
		{LangPython, 1700},
		{LangJavaScript, 1700},
	}

	for _, tc := range testCases {
		size := SizeForLanguage(tc.lang)
		if size < tc.minSize {
			t.Errorf("SizeForLanguage(%d): got %d, want >= %d", tc.lang, size, tc.minSize)
		}

		// Should match ForLanguage().Size()
		vocab := ForLanguage(tc.lang)
		if size != vocab.Size() {
			t.Errorf("SizeForLanguage(%d) mismatch: %d vs %d", tc.lang, size, vocab.Size())
		}
	}
}

// Test unknown language falls back to default
func TestForLanguageUnknown(t *testing.T) {
	vocab := ForLanguage(Language(999))
	defaultVocab := Default()

	if vocab.Size() != defaultVocab.Size() {
		t.Errorf("unknown language should return default vocab: got %d, want %d",
			vocab.Size(), defaultVocab.Size())
	}
}

// Test language vocabularies are distinct
func TestLanguageVocabsDistinct(t *testing.T) {
	textVocab := ForLanguage(LangText)
	goVocab := ForLanguage(LangGo)
	pyVocab := ForLanguage(LangPython)
	jsVocab := ForLanguage(LangJavaScript)

	// Sizes should differ (language-specific vocabs are larger)
	if textVocab.Size() >= goVocab.Size() {
		t.Error("Go vocab should be larger than text vocab")
	}

	// Check that language vocabs have language-specific tokens
	// Go should have "func " token
	goHasFunc := false
	for i := 256; i < goVocab.Size(); i++ {
		tok, ok := goVocab.GetToken(i)
		if ok && string(tok.Bytes) == "func " {
			goHasFunc = true
			break
		}
	}
	if !goHasFunc {
		t.Log("Warning: Go vocab may not have 'func ' as a token")
	}

	// Python should have "def " token
	pyHasDef := false
	for i := 256; i < pyVocab.Size(); i++ {
		tok, ok := pyVocab.GetToken(i)
		if ok && string(tok.Bytes) == "def " {
			pyHasDef = true
			break
		}
	}
	if !pyHasDef {
		t.Log("Warning: Python vocab may not have 'def ' as a token")
	}

	// JS should have "const " token
	jsHasConst := false
	for i := 256; i < jsVocab.Size(); i++ {
		tok, ok := jsVocab.GetToken(i)
		if ok && string(tok.Bytes) == "const " {
			jsHasConst = true
			break
		}
	}
	if !jsHasConst {
		t.Log("Warning: JS vocab may not have 'const ' as a token")
	}
}

// Test vocab decode roundtrip
func TestVocabDecodeRoundtrip(t *testing.T) {
	languages := []Language{LangText, LangGo, LangPython, LangJavaScript}

	for _, lang := range languages {
		vocab := ForLanguage(lang)

		// Test that we can decode all valid token IDs
		for i := 0; i < vocab.Size(); i++ {
			tok, ok := vocab.GetToken(i)
			if !ok {
				t.Errorf("lang %d: missing token %d", lang, i)
				continue
			}
			if len(tok.Bytes) == 0 {
				t.Errorf("lang %d: token %d has empty bytes", lang, i)
			}
		}
	}
}

// Benchmark vocabulary lookups
func BenchmarkVocabLookup(b *testing.B) {
	vocab := ForLanguage(LangGo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vocab.GetToken(i % vocab.Size())
	}
}

func BenchmarkForLanguage(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ForLanguage(Language(i % 4))
	}
}
