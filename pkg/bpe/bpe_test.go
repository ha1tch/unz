package bpe

import (
	"bytes"
	"strings"
	"testing"
)

func TestVocabularyBasic(t *testing.T) {
	tokens := map[string]int{
		"a": 0,
		"b": 1,
		"c": 2,
	}
	vocab := NewVocabulary(tokens)

	if vocab.Size() != 3 {
		t.Errorf("size: got %d, want 3", vocab.Size())
	}

	// Test GetToken
	tok, ok := vocab.GetToken(0)
	if !ok || string(tok.Bytes) != "a" {
		t.Errorf("GetToken(0): got %q, want 'a'", tok.Bytes)
	}

	// Test GetID
	id, ok := vocab.GetID([]byte("b"))
	if !ok || id != 1 {
		t.Errorf("GetID('b'): got %d, want 1", id)
	}

	// Test non-existent
	_, ok = vocab.GetToken(99)
	if ok {
		t.Error("GetToken(99) should return false")
	}

	_, ok = vocab.GetID([]byte("xyz"))
	if ok {
		t.Error("GetID('xyz') should return false")
	}
}

func TestVocabularyDecode(t *testing.T) {
	tokens := map[string]int{
		"h":  0,
		"e":  1,
		"l":  2,
		"o":  3,
		" ":  4,
		"he": 5,
		"ll": 6,
	}
	vocab := NewVocabulary(tokens)

	testCases := []struct {
		ids  []int
		want string
	}{
		{[]int{}, ""},
		{[]int{0, 1, 2, 2, 3}, "hello"},
		{[]int{5, 6, 3}, "hello"},
		{[]int{0, 1, 2, 2, 3, 4}, "hello "},
	}

	for _, tc := range testCases {
		got := string(vocab.Decode(tc.ids))
		if got != tc.want {
			t.Errorf("Decode(%v): got %q, want %q", tc.ids, got, tc.want)
		}
	}
}

func TestVocabularyAllTokens(t *testing.T) {
	tokens := map[string]int{
		"a":  0,
		"b":  1,
		"ab": 2,
	}
	vocab := NewVocabulary(tokens)

	all := vocab.AllTokens()
	if len(all) != 3 {
		t.Errorf("AllTokens length: got %d, want 3", len(all))
	}

	for tok, id := range tokens {
		if gotID, ok := all[tok]; !ok || gotID != id {
			t.Errorf("AllTokens[%q]: got %d, want %d", tok, gotID, id)
		}
	}
}

func TestCreateBasicVocab(t *testing.T) {
	vocab := CreateBasicVocab()

	if vocab.Size() != 256 {
		t.Errorf("size: got %d, want 256", vocab.Size())
	}

	// Check all bytes are present
	for i := 0; i < 256; i++ {
		tok, ok := vocab.GetToken(i)
		if !ok {
			t.Errorf("missing token for byte %d", i)
			continue
		}
		if len(tok.Bytes) != 1 || tok.Bytes[0] != byte(i) {
			t.Errorf("token %d: got %v, want [%d]", i, tok.Bytes, i)
		}
	}
}

func TestTrain(t *testing.T) {
	text := []byte("aaabbbaaabbb")
	vocab := Train(text, 5)

	// Should have base bytes plus some merges
	if vocab.Size() < 256 {
		t.Errorf("size too small: got %d", vocab.Size())
	}

	// Should be able to encode/decode
	encoder := NewEncoder(vocab)
	ids := encoder.Encode(text)
	decoded := encoder.Decode(ids)

	if !bytes.Equal(decoded, text) {
		t.Errorf("roundtrip failed: got %q, want %q", decoded, text)
	}
}

func TestFastTrieBasic(t *testing.T) {
	trie := NewFastTrie()

	trie.Insert([]byte("hello"), 1)
	trie.Insert([]byte("help"), 2)
	trie.Insert([]byte("he"), 3)

	testCases := []struct {
		input   string
		wantLen int
		wantID  int
	}{
		{"hello world", 5, 1},
		{"help me", 4, 2},
		{"he said", 2, 3},
		{"hero", 2, 3}, // "he" is longest match
		{"hi", 0, -1},  // no match
	}

	for _, tc := range testCases {
		gotLen, gotID := trie.LongestMatch([]byte(tc.input))
		if gotLen != tc.wantLen || gotID != tc.wantID {
			t.Errorf("LongestMatch(%q): got (%d, %d), want (%d, %d)",
				tc.input, gotLen, gotID, tc.wantLen, tc.wantID)
		}
	}
}

func TestEncoderBasic(t *testing.T) {
	tokens := map[string]int{}
	for i := 0; i < 256; i++ {
		tokens[string([]byte{byte(i)})] = i
	}
	tokens["th"] = 256
	tokens["he"] = 257
	tokens["the"] = 258

	vocab := NewVocabulary(tokens)
	encoder := NewEncoder(vocab)

	text := []byte("the")
	ids := encoder.Encode(text)

	// Should use longest match "the"
	if len(ids) != 1 || ids[0] != 258 {
		t.Errorf("Encode('the'): got %v, want [258]", ids)
	}

	decoded := encoder.Decode(ids)
	if !bytes.Equal(decoded, text) {
		t.Errorf("Decode: got %q, want %q", decoded, text)
	}
}

func TestEncoderRoundtrip(t *testing.T) {
	vocab := CreateBasicVocab()
	encoder := NewEncoder(vocab)

	testCases := []string{
		"",
		"a",
		"hello",
		"Hello, World!",
		"the quick brown fox",
		"\x00\x01\x02\xff",
		strings.Repeat("abc", 100),
	}

	for _, text := range testCases {
		t.Run(text[:min(len(text), 20)], func(t *testing.T) {
			data := []byte(text)
			ids := encoder.Encode(data)
			decoded := encoder.Decode(ids)

			if !bytes.Equal(decoded, data) {
				t.Errorf("roundtrip failed for %q", text)
			}
		})
	}
}

func TestEncoderVocabulary(t *testing.T) {
	vocab := CreateBasicVocab()
	encoder := NewEncoder(vocab)

	if encoder.Vocabulary() != vocab {
		t.Error("Vocabulary() should return original vocab")
	}
}

func TestVocabularyMaxLen(t *testing.T) {
	tokens := map[string]int{
		"a":     0,
		"bb":    1,
		"ccc":   2,
		"dddd":  3,
		"eeeee": 4,
	}
	vocab := NewVocabulary(tokens)

	if vocab.MaxLen() != 5 {
		t.Errorf("MaxLen: got %d, want 5", vocab.MaxLen())
	}
}

func BenchmarkEncode(b *testing.B) {
	vocab := Train([]byte(strings.Repeat("the quick brown fox ", 100)), 500)
	encoder := NewEncoder(vocab)
	text := []byte(strings.Repeat("the quick brown fox ", 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Encode(text)
	}
}

func BenchmarkDecode(b *testing.B) {
	vocab := Train([]byte(strings.Repeat("the quick brown fox ", 100)), 500)
	encoder := NewEncoder(vocab)
	text := []byte(strings.Repeat("the quick brown fox ", 1000))
	ids := encoder.Encode(text)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Decode(ids)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
