// Package ans provides entropy coding using rANS (range Asymmetric Numeral Systems).
//
// Includes both single-threaded and parallel implementations for large data.
package ans

import (
	"encoding/binary"
	"errors"
	"runtime"
	"sync"
)

const (
	ProbBits  = 14
	ProbScale = 1 << ProbBits
	RansL     = 1 << 23
)

var (
	ErrEmpty     = errors.New("ans: empty input")
	ErrCorrupted = errors.New("ans: corrupted data")
)

// Symbol contains frequency information for encoding/decoding.
type Symbol struct {
	CumFreq uint32
	Freq    uint32
}

// SymbolTable holds encode/decode tables.
type SymbolTable struct {
	Symbols  [256]Symbol
	CumToSym [ProbScale]uint16
}

// BuildTable creates a symbol table from frequency counts.
func BuildTable(counts []uint32) *SymbolTable {
	tab := &SymbolTable{}

	// Calculate total and normalize
	var total uint64
	for _, c := range counts {
		total += uint64(c)
	}
	if total == 0 {
		tab.Symbols[0] = Symbol{Freq: ProbScale}
		for i := range tab.CumToSym {
			tab.CumToSym[i] = 0
		}
		return tab
	}

	// Normalize to ProbScale
	normalized := [256]uint32{}
	var normTotal uint32
	for i, c := range counts {
		if c == 0 {
			continue
		}
		n := uint32((uint64(c) * ProbScale) / total)
		if n == 0 {
			n = 1
		}
		normalized[i] = n
		normTotal += n
	}

	// Adjust largest to match exactly
	if normTotal != ProbScale {
		maxIdx := 0
		for i, n := range normalized {
			if n > normalized[maxIdx] {
				maxIdx = i
			}
		}
		if normTotal > ProbScale {
			normalized[maxIdx] -= normTotal - ProbScale
		} else {
			normalized[maxIdx] += ProbScale - normTotal
		}
	}

	// Build cumulative and lookup
	var cumFreq uint32
	for i, n := range normalized {
		tab.Symbols[i] = Symbol{CumFreq: cumFreq, Freq: n}
		for j := uint32(0); j < n; j++ {
			tab.CumToSym[cumFreq+j] = uint16(i)
		}
		cumFreq += n
	}

	return tab
}

// === ENCODER ===

// Encoder encodes symbols using rANS.
type Encoder struct {
	state  uint32
	output []byte
}

// NewEncoder creates a new encoder.
func NewEncoder() *Encoder {
	return &Encoder{state: RansL}
}

// Reset resets the encoder for reuse.
func (e *Encoder) Reset() {
	e.state = RansL
	e.output = e.output[:0]
}

// Encode encodes a single symbol.
func (e *Encoder) Encode(sym byte, tab *SymbolTable) {
	s := &tab.Symbols[sym]
	freq := s.Freq
	if freq == 0 {
		return
	}

	// Renormalize
	maxState := ((RansL >> ProbBits) << 8) * freq
	for e.state >= maxState {
		e.output = append(e.output, byte(e.state))
		e.state >>= 8
	}

	// Encode
	e.state = ((e.state / freq) << ProbBits) + s.CumFreq + (e.state % freq)
}

// Finish finalizes encoding and returns the compressed data.
func (e *Encoder) Finish() []byte {
	// State as 4 bytes
	stateBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(stateBytes, e.state)

	// Reverse output
	for i, j := 0, len(e.output)-1; i < j; i, j = i+1, j-1 {
		e.output[i], e.output[j] = e.output[j], e.output[i]
	}

	result := make([]byte, 4+len(e.output))
	copy(result[:4], stateBytes)
	copy(result[4:], e.output)
	return result
}

// === DECODER ===

// Decoder decodes symbols using rANS.
type Decoder struct {
	state uint32
	data  []byte
	pos   int
}

// NewDecoder creates a decoder from compressed data.
func NewDecoder(data []byte) (*Decoder, error) {
	if len(data) < 4 {
		return nil, ErrCorrupted
	}
	return &Decoder{
		state: binary.LittleEndian.Uint32(data[:4]),
		data:  data,
		pos:   4,
	}, nil
}

// Decode decodes a single symbol.
func (d *Decoder) Decode(tab *SymbolTable) byte {
	cumFreq := d.state & (ProbScale - 1)
	sym := tab.CumToSym[cumFreq]
	s := &tab.Symbols[sym]

	// Decode step
	d.state = s.Freq*(d.state>>ProbBits) + cumFreq - s.CumFreq

	// Renormalize
	for d.state < RansL && d.pos < len(d.data) {
		d.state = (d.state << 8) | uint32(d.data[d.pos])
		d.pos++
	}

	return byte(sym)
}

// === SINGLE-THREADED API ===

// Compress compresses data using rANS.
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{0, 0, 0, 0}, nil
	}

	// Count frequencies
	counts := make([]uint32, 256)
	for _, b := range data {
		counts[b]++
	}

	tab := BuildTable(counts)

	// Encode in reverse
	enc := NewEncoder()
	for i := len(data) - 1; i >= 0; i-- {
		enc.Encode(data[i], tab)
	}
	compressed := enc.Finish()

	// Output: [len:4][freqs:256*2][compressed]
	output := make([]byte, 4+256*2+len(compressed))
	binary.LittleEndian.PutUint32(output[:4], uint32(len(data)))
	for i := 0; i < 256; i++ {
		binary.LittleEndian.PutUint16(output[4+i*2:], uint16(tab.Symbols[i].Freq))
	}
	copy(output[4+256*2:], compressed)

	return output, nil
}

// Decompress decompresses rANS-compressed data.
func Decompress(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, ErrCorrupted
	}

	origLen := int(binary.LittleEndian.Uint32(data[:4]))
	if origLen == 0 {
		return []byte{}, nil
	}

	if len(data) < 4+256*2+4 {
		return nil, ErrCorrupted
	}

	// Rebuild table from frequencies
	counts := make([]uint32, 256)
	for i := 0; i < 256; i++ {
		counts[i] = uint32(binary.LittleEndian.Uint16(data[4+i*2:]))
	}
	tab := BuildTable(counts)

	dec, err := NewDecoder(data[4+256*2:])
	if err != nil {
		return nil, err
	}

	output := make([]byte, origLen)
	for i := 0; i < origLen; i++ {
		output[i] = dec.Decode(tab)
	}

	return output, nil
}

// === PARALLEL API ===

const (
	DefaultChunkSize = 64 * 1024
	MinChunkSize     = 4 * 1024
)

// CompressParallel compresses using multiple goroutines.
func CompressParallel(data []byte, chunkSize int) ([]byte, error) {
	if len(data) == 0 {
		return []byte{0, 0, 0, 0, 0, 0, 0, 0}, nil
	}

	if chunkSize < MinChunkSize {
		chunkSize = MinChunkSize
	}

	numChunks := (len(data) + chunkSize - 1) / chunkSize
	workers := runtime.GOMAXPROCS(0)

	type chunkResult struct {
		compressed []byte
		origSize   int
		err        error
	}
	results := make([]chunkResult, numChunks)

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for i := 0; i < numChunks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := idx * chunkSize
			end := start + chunkSize
			if end > len(data) {
				end = len(data)
			}
			chunkData := data[start:end]

			compressed, err := Compress(chunkData)
			results[idx] = chunkResult{
				compressed: compressed,
				origSize:   len(chunkData),
				err:        err,
			}
		}(i)
	}
	wg.Wait()

	// Check errors
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
	}

	// Header: origLen(4) + numChunks(4) + [origSize(4) + compSize(4)] * numChunks
	headerSize := 8 + numChunks*8
	totalSize := headerSize
	for _, r := range results {
		totalSize += len(r.compressed)
	}

	output := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(output[0:], uint32(len(data)))
	binary.LittleEndian.PutUint32(output[4:], uint32(numChunks))

	pos := 8
	for i := 0; i < numChunks; i++ {
		binary.LittleEndian.PutUint32(output[pos:], uint32(results[i].origSize))
		binary.LittleEndian.PutUint32(output[pos+4:], uint32(len(results[i].compressed)))
		pos += 8
	}
	for i := 0; i < numChunks; i++ {
		copy(output[pos:], results[i].compressed)
		pos += len(results[i].compressed)
	}

	return output, nil
}

// DecompressParallel decompresses using multiple goroutines.
func DecompressParallel(data []byte) ([]byte, error) {
	if len(data) < 8 {
		return nil, ErrCorrupted
	}

	origLen := int(binary.LittleEndian.Uint32(data[0:]))
	if origLen == 0 {
		return []byte{}, nil
	}

	numChunks := int(binary.LittleEndian.Uint32(data[4:]))
	if len(data) < 8+numChunks*8 {
		return nil, ErrCorrupted
	}

	// Read chunk sizes
	type chunkInfo struct {
		origSize int
		compSize int
	}
	chunks := make([]chunkInfo, numChunks)
	pos := 8
	for i := 0; i < numChunks; i++ {
		chunks[i].origSize = int(binary.LittleEndian.Uint32(data[pos:]))
		chunks[i].compSize = int(binary.LittleEndian.Uint32(data[pos+4:]))
		pos += 8
	}

	output := make([]byte, origLen)
	workers := runtime.GOMAXPROCS(0)

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	errCh := make(chan error, numChunks)

	outPos := 0
	dataPos := pos
	for i := 0; i < numChunks; i++ {
		chunkData := data[dataPos : dataPos+chunks[i].compSize]
		chunkOutStart := outPos

		wg.Add(1)
		go func(chunk []byte, outStart int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dec, err := Decompress(chunk)
			if err != nil {
				errCh <- err
				return
			}
			copy(output[outStart:], dec)
		}(chunkData, chunkOutStart)

		dataPos += chunks[i].compSize
		outPos += chunks[i].origSize
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return nil, err
	}

	return output, nil
}
