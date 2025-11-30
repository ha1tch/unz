// Package bpe implements Byte Pair Encoding tokenization for text compression.
//
// This provides O(n) tokenization using a FastTrie for efficient longest-prefix matching.
// Compatible with tiktoken vocabulary files.
package bpe

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Token represents a token in the vocabulary.
type Token struct {
	Bytes []byte // The actual bytes of the token
	Rank  int    // Priority rank (lower = merge earlier)
}

// Vocabulary holds the BPE vocabulary.
type Vocabulary struct {
	tokens   []Token        // Indexed by token ID
	byteToID map[string]int // Token bytes -> token ID
	maxLen   int            // Maximum token length in bytes
}

// NewVocabulary creates a new vocabulary from a map of token bytes to ranks.
func NewVocabulary(tokenRanks map[string]int) *Vocabulary {
	v := &Vocabulary{
		tokens:   make([]Token, len(tokenRanks)),
		byteToID: make(map[string]int, len(tokenRanks)),
	}

	// Sort by rank to assign IDs in rank order
	type tokenRank struct {
		bytes []byte
		rank  int
	}
	sorted := make([]tokenRank, 0, len(tokenRanks))
	for b, r := range tokenRanks {
		sorted = append(sorted, tokenRank{[]byte(b), r})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].rank < sorted[j].rank
	})

	// Assign token IDs in rank order
	for id, tr := range sorted {
		v.tokens[id] = Token{
			Bytes: tr.bytes,
			Rank:  tr.rank,
		}
		v.byteToID[string(tr.bytes)] = id
		if len(tr.bytes) > v.maxLen {
			v.maxLen = len(tr.bytes)
		}
	}

	return v
}

// Size returns the vocabulary size.
func (v *Vocabulary) Size() int {
	return len(v.tokens)
}

// MaxLen returns the maximum token length in bytes.
func (v *Vocabulary) MaxLen() int {
	return v.maxLen
}

// GetToken returns the token for a given ID.
func (v *Vocabulary) GetToken(id int) (Token, bool) {
	if id < 0 || id >= len(v.tokens) {
		return Token{}, false
	}
	return v.tokens[id], true
}

// GetID returns the ID for given token bytes.
func (v *Vocabulary) GetID(bytes []byte) (int, bool) {
	id, ok := v.byteToID[string(bytes)]
	return id, ok
}

// Decode converts token IDs back to bytes.
func (v *Vocabulary) Decode(ids []int) []byte {
	// Calculate total length
	total := 0
	for _, id := range ids {
		if id >= 0 && id < len(v.tokens) {
			total += len(v.tokens[id].Bytes)
		}
	}

	result := make([]byte, 0, total)
	for _, id := range ids {
		if id >= 0 && id < len(v.tokens) {
			result = append(result, v.tokens[id].Bytes...)
		}
	}
	return result
}

// AllTokens returns a map of token string to token ID.
func (v *Vocabulary) AllTokens() map[string]int {
	result := make(map[string]int, len(v.tokens))
	for id, tok := range v.tokens {
		result[string(tok.Bytes)] = id
	}
	return result
}

// LoadTiktoken loads a vocabulary from a tiktoken-format reader.
// Format: base64-encoded token bytes followed by space and rank.
func LoadTiktoken(r io.Reader) (*Vocabulary, error) {
	tokenRanks := make(map[string]int)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		tokenBytes, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid base64: %s", parts[0])
		}

		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid rank: %s", parts[1])
		}

		tokenRanks[string(tokenBytes)] = rank
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return NewVocabulary(tokenRanks), nil
}

// CreateBasicVocab creates a basic 256-byte vocabulary (no merges).
func CreateBasicVocab() *Vocabulary {
	tokenRanks := make(map[string]int)
	for i := 0; i < 256; i++ {
		tokenRanks[string([]byte{byte(i)})] = i
	}
	return NewVocabulary(tokenRanks)
}

// Train trains a BPE vocabulary on the given text.
// numMerges specifies how many merge operations to perform.
func Train(text []byte, numMerges int) *Vocabulary {
	// Start with byte-level tokens
	tokenRanks := make(map[string]int)
	for i := 0; i < 256; i++ {
		tokenRanks[string([]byte{byte(i)})] = i
	}

	// Convert text to token IDs
	ids := make([]int, len(text))
	for i, b := range text {
		ids[i] = int(b)
	}

	nextRank := 256

	for merge := 0; merge < numMerges; merge++ {
		// Count pairs
		pairCounts := make(map[string]int)
		for i := 0; i < len(ids)-1; i++ {
			key := fmt.Sprintf("%d,%d", ids[i], ids[i+1])
			pairCounts[key]++
		}

		if len(pairCounts) == 0 {
			break
		}

		// Find most frequent pair
		var bestPair string
		bestCount := 0
		for pair, count := range pairCounts {
			if count > bestCount {
				bestCount = count
				bestPair = pair
			}
		}

		if bestCount < 2 {
			break // No more useful merges
		}

		// Parse the pair
		parts := strings.Split(bestPair, ",")
		id1, _ := strconv.Atoi(parts[0])
		id2, _ := strconv.Atoi(parts[1])

		// Find the bytes for this new token
		var newBytes []byte
		for b, r := range tokenRanks {
			if r == id1 {
				newBytes = append(newBytes, []byte(b)...)
				break
			}
		}
		for b, r := range tokenRanks {
			if r == id2 {
				newBytes = append(newBytes, []byte(b)...)
				break
			}
		}

		// Add new token
		tokenRanks[string(newBytes)] = nextRank
		newID := nextRank
		nextRank++

		// Merge in the ID sequence
		newIDs := make([]int, 0, len(ids))
		i := 0
		for i < len(ids) {
			if i < len(ids)-1 && ids[i] == id1 && ids[i+1] == id2 {
				newIDs = append(newIDs, newID)
				i += 2
			} else {
				newIDs = append(newIDs, ids[i])
				i++
			}
		}
		ids = newIDs
	}

	return NewVocabulary(tokenRanks)
}
