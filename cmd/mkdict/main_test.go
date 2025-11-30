package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestTrainBPE(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		numMerges int
		minTokens int
		maxTokens int
	}{
		{
			name:      "basic text",
			input:     "the quick brown fox jumps over the lazy dog",
			numMerges: 10,
			minTokens: 256,      // at least byte tokens
			maxTokens: 256 + 10, // at most 10 merges
		},
		{
			name:      "repetitive text",
			input:     "abababababababababab",
			numMerges: 5,
			minTokens: 256,
			maxTokens: 261,
		},
		{
			name:      "no merges",
			input:     "abc",
			numMerges: 0,
			minTokens: 256,
			maxTokens: 256,
		},
		{
			name:      "empty input returns basic vocab",
			input:     "",
			numMerges: 100,
			minTokens: 0,
			maxTokens: 256,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenRanks := trainBPE([]byte(tc.input), tc.numMerges)

			if len(tokenRanks) < tc.minTokens {
				t.Errorf("too few tokens: got %d, want >= %d", len(tokenRanks), tc.minTokens)
			}

			if len(tokenRanks) > tc.maxTokens {
				t.Errorf("too many tokens: got %d, want <= %d", len(tokenRanks), tc.maxTokens)
			}
		})
	}
}

func TestTrainBPEByteTokens(t *testing.T) {
	// All 256 byte values should be present
	input := make([]byte, 256)
	for i := range input {
		input[i] = byte(i)
	}

	tokenRanks := trainBPE(input, 0)

	// Check all single-byte tokens exist
	for i := 0; i < 256; i++ {
		key := string([]byte{byte(i)})
		if _, ok := tokenRanks[key]; !ok {
			t.Errorf("missing byte token for %d", i)
		}
	}
}

func TestTrainBPEMerges(t *testing.T) {
	// Repetitive input should create merged tokens
	input := []byte("aaaa bbbb aaaa bbbb aaaa bbbb")
	tokenRanks := trainBPE(input, 10)

	// Should have merged "aa" and "bb"
	hasAA := false
	hasBB := false
	for token := range tokenRanks {
		if token == "aa" {
			hasAA = true
		}
		if token == "bb" {
			hasBB = true
		}
	}

	if !hasAA {
		t.Error("expected 'aa' to be merged")
	}
	if !hasBB {
		t.Error("expected 'bb' to be merged")
	}
}

func TestGoStringLiteral(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"`},
		{"", `""`},
		{"a b", `"a b"`},
		{"a\nb", `"a\nb"`},
		{"a\tb", `"a\tb"`},
		{"a\rb", `"a\rb"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"a\x00b", `"a\x00b"`},
		{"a\xffb", `"a\xffb"`},
		{"\x01\x02\x03", `"\x01\x02\x03"`},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := goStringLiteral(tc.input)
			if result != tc.expected {
				t.Errorf("got %s, want %s", result, tc.expected)
			}
		})
	}
}

func TestWriteGoSource(t *testing.T) {
	tokenRanks := map[string]int{
		"a":  0,
		"b":  1,
		"ab": 2,
	}

	var buf bytes.Buffer

	// Save original flags and restore
	origPkg := *goPackage
	origVar := *varName
	origMerges := *numMerges
	defer func() {
		*goPackage = origPkg
		*varName = origVar
		*numMerges = origMerges
	}()

	*goPackage = "testvocab"
	*varName = "testTokens"
	*numMerges = 2

	writeGoSource(&buf, tokenRanks)

	output := buf.String()

	// Check package declaration
	if !strings.Contains(output, "package testvocab") {
		t.Error("missing package declaration")
	}

	// Check variable name
	if !strings.Contains(output, "var testTokens = map[string]int{") {
		t.Error("missing variable declaration")
	}

	// Check tokens are present
	if !strings.Contains(output, `"a": 0,`) {
		t.Error("missing token 'a'")
	}
	if !strings.Contains(output, `"b": 1,`) {
		t.Error("missing token 'b'")
	}
	if !strings.Contains(output, `"ab": 2,`) {
		t.Error("missing token 'ab'")
	}

	// Check it's valid Go (starts with comment, has closing brace)
	if !strings.HasPrefix(output, "// Code generated") {
		t.Error("missing generated comment")
	}
	if !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Error("missing closing brace")
	}
}

func TestTrainBPEConsistentSize(t *testing.T) {
	// Same input should produce same number of tokens
	// (exact tokens may vary due to tie-breaking in pair selection)
	input := []byte("the quick brown fox the quick brown fox")

	result1 := trainBPE(input, 20)
	result2 := trainBPE(input, 20)

	if len(result1) != len(result2) {
		t.Errorf("inconsistent sizes: got %d and %d tokens", len(result1), len(result2))
	}

	// Both should have all base byte tokens
	for i := 0; i < 256; i++ {
		key := string([]byte{byte(i)})
		if _, ok := result1[key]; !ok {
			t.Errorf("result1 missing byte token %d", i)
		}
		if _, ok := result2[key]; !ok {
			t.Errorf("result2 missing byte token %d", i)
		}
	}
}

func TestTrainBPEMergeCount(t *testing.T) {
	// With enough merges, should reduce token count
	input := bytes.Repeat([]byte("hello world "), 100)

	tokens10 := trainBPE(input, 10)
	tokens100 := trainBPE(input, 100)

	// More merges = more tokens in vocabulary
	if len(tokens100) <= len(tokens10) {
		t.Errorf("more merges should create more tokens: %d vs %d", len(tokens100), len(tokens10))
	}
}

func TestTrainBPEUnicode(t *testing.T) {
	// Test with UTF-8 text
	input := []byte("héllo wörld 你好世界")
	tokenRanks := trainBPE(input, 10)

	// Should have at least base byte tokens
	if len(tokenRanks) < 256 {
		t.Errorf("missing byte tokens: got %d", len(tokenRanks))
	}

	// Reconstruct and verify all bytes are representable
	for _, b := range input {
		key := string([]byte{b})
		if _, ok := tokenRanks[key]; !ok {
			t.Errorf("missing byte token for 0x%02x", b)
		}
	}
}

func BenchmarkTrainBPE(b *testing.B) {
	input := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trainBPE(input, 500)
	}
}
