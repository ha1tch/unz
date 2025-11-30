package ans

import (
	"bytes"
	"testing"
)

func TestCompressDecompressRoundtrip(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short", []byte("hello")},
		{"longer", []byte("the quick brown fox jumps over the lazy dog")},
		{"repetitive", bytes.Repeat([]byte{0xAA}, 1000)},
		{"all same", bytes.Repeat([]byte{0}, 100)},
		{"all bytes", makeAllBytes()},
		{"random-ish", makeRandomish(1000)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := Compress(tc.data)
			if err != nil {
				t.Fatalf("Compress failed: %v", err)
			}

			decompressed, err := Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}

			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("roundtrip failed: got %d bytes, want %d bytes",
					len(decompressed), len(tc.data))
			}
		})
	}
}

func TestCompressParallelRoundtrip(t *testing.T) {
	data := bytes.Repeat([]byte("the quick brown fox "), 10000)

	compressed, err := CompressParallel(data, DefaultChunkSize)
	if err != nil {
		t.Fatalf("CompressParallel failed: %v", err)
	}

	decompressed, err := DecompressParallel(compressed)
	if err != nil {
		t.Fatalf("DecompressParallel failed: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("parallel roundtrip failed: got %d bytes, want %d bytes",
			len(decompressed), len(data))
	}
}

func TestCompressEmpty(t *testing.T) {
	compressed, err := Compress([]byte{})
	if err != nil {
		t.Fatalf("Compress empty failed: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress empty failed: %v", err)
	}

	if len(decompressed) != 0 {
		t.Errorf("expected empty, got %d bytes", len(decompressed))
	}
}

func TestDecompressInvalid(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"too short", []byte{0, 0, 0}},
		{"truncated header", []byte{10, 0, 0, 0}}, // claims 10 bytes but no freq table
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decompress(tc.data)
			if err == nil {
				t.Error("expected error for invalid data")
			}
		})
	}
}

func TestBuildTable(t *testing.T) {
	// Uniform distribution
	counts := make([]uint32, 256)
	for i := range counts {
		counts[i] = 100
	}

	tab := BuildTable(counts)

	// Check frequencies sum to ProbScale
	var sum uint32
	for i := 0; i < 256; i++ {
		sum += tab.Symbols[i].Freq
	}
	if sum != ProbScale {
		t.Errorf("frequencies sum to %d, want %d", sum, ProbScale)
	}

	// Check cumulative frequencies
	var cum uint32
	for i := 0; i < 256; i++ {
		if tab.Symbols[i].CumFreq != cum {
			t.Errorf("symbol %d: CumFreq = %d, want %d", i, tab.Symbols[i].CumFreq, cum)
		}
		cum += tab.Symbols[i].Freq
	}
}

func TestBuildTableSkewed(t *testing.T) {
	// Highly skewed distribution
	counts := make([]uint32, 256)
	counts[0] = 10000 // One very common symbol
	for i := 1; i < 256; i++ {
		counts[i] = 1
	}

	tab := BuildTable(counts)

	// Symbol 0 should have most of the probability
	if tab.Symbols[0].Freq < ProbScale/2 {
		t.Errorf("symbol 0 freq too low: %d", tab.Symbols[0].Freq)
	}

	// Rare symbols should still have at least 1
	for i := 1; i < 256; i++ {
		if counts[i] > 0 && tab.Symbols[i].Freq == 0 {
			t.Errorf("symbol %d has zero freq despite non-zero count", i)
		}
	}
}

func TestBuildTableEmpty(t *testing.T) {
	counts := make([]uint32, 256)
	tab := BuildTable(counts)

	// Should handle all-zero gracefully
	if tab.Symbols[0].Freq != ProbScale {
		t.Errorf("expected symbol 0 to get all probability for empty counts")
	}
}

func TestEncoderDecoder(t *testing.T) {
	counts := make([]uint32, 256)
	data := []byte("hello world")
	for _, b := range data {
		counts[b]++
	}

	tab := BuildTable(counts)

	// Encode
	enc := NewEncoder()
	for i := len(data) - 1; i >= 0; i-- {
		enc.Encode(data[i], tab)
	}
	compressed := enc.Finish()

	// Decode
	dec, err := NewDecoder(compressed)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	decoded := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		decoded[i] = dec.Decode(tab)
	}

	if !bytes.Equal(decoded, data) {
		t.Errorf("encode/decode failed: got %q, want %q", decoded, data)
	}
}

func TestEncoderReset(t *testing.T) {
	enc := NewEncoder()

	// First encoding
	counts := make([]uint32, 256)
	counts['a'] = 100
	tab := BuildTable(counts)

	enc.Encode('a', tab)
	first := enc.Finish()

	// Reset and encode again
	enc.Reset()
	enc.Encode('a', tab)
	second := enc.Finish()

	if !bytes.Equal(first, second) {
		t.Error("Reset should produce same results")
	}
}

func TestChunkSize(t *testing.T) {
	data := bytes.Repeat([]byte("test"), 1000)

	// Test with various chunk sizes
	chunkSizes := []int{
		MinChunkSize,
		DefaultChunkSize,
		MinChunkSize / 2, // Should be bumped up to MinChunkSize
	}

	for _, chunkSize := range chunkSizes {
		compressed, err := CompressParallel(data, chunkSize)
		if err != nil {
			t.Errorf("CompressParallel(chunkSize=%d) failed: %v", chunkSize, err)
			continue
		}

		decompressed, err := DecompressParallel(compressed)
		if err != nil {
			t.Errorf("DecompressParallel(chunkSize=%d) failed: %v", chunkSize, err)
			continue
		}

		if !bytes.Equal(decompressed, data) {
			t.Errorf("roundtrip failed with chunkSize=%d", chunkSize)
		}
	}
}

func TestCompressRatio(t *testing.T) {
	// Highly compressible data
	repetitive := bytes.Repeat([]byte{0xAA}, 10000)
	compressed, _ := Compress(repetitive)

	ratio := float64(len(compressed)) / float64(len(repetitive))
	if ratio > 0.1 {
		t.Errorf("poor compression for repetitive data: %.2f", ratio)
	}

	// Less compressible data
	varied := makeAllBytes()
	varied = append(varied, varied...)
	varied = append(varied, varied...)
	compressed, _ = Compress(varied)

	// Should still be close to original for uniform distribution
	// ANS has some overhead for frequency table (512 bytes) + state
	ratio = float64(len(compressed)) / float64(len(varied))
	if ratio > 2.0 { // Allow overhead for small uniform data
		t.Errorf("excessive expansion for varied data: %.2f", ratio)
	}
}

func BenchmarkCompress(b *testing.B) {
	data := bytes.Repeat([]byte("the quick brown fox "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compress(data)
	}
}

func BenchmarkDecompress(b *testing.B) {
	data := bytes.Repeat([]byte("the quick brown fox "), 1000)
	compressed, _ := Compress(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decompress(compressed)
	}
}

func BenchmarkCompressParallel(b *testing.B) {
	data := bytes.Repeat([]byte("the quick brown fox "), 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompressParallel(data, DefaultChunkSize)
	}
}

func makeAllBytes() []byte {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}

func makeRandomish(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i*179 + 83) % 256)
	}
	return data
}
