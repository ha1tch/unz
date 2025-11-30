package bpe

// FastTrie is a trie optimized for BPE tokenization.
// Uses a simple node-per-byte structure for fast character-by-character traversal.
type FastTrie struct {
	root *fastTrieNode
}

type fastTrieNode struct {
	children [256]*fastTrieNode // direct array for O(1) child lookup
	tokenID  int                // -1 if not a token endpoint
	isToken  bool
}

// NewFastTrie creates a new fast trie.
func NewFastTrie() *FastTrie {
	return &FastTrie{
		root: &fastTrieNode{tokenID: -1},
	}
}

// Insert adds a token to the trie.
func (t *FastTrie) Insert(token []byte, id int) {
	node := t.root
	for _, b := range token {
		if node.children[b] == nil {
			node.children[b] = &fastTrieNode{tokenID: -1}
		}
		node = node.children[b]
	}
	node.tokenID = id
	node.isToken = true
}

// LongestMatch finds the longest token matching a prefix of the input.
// Returns (length, tokenID) or (0, -1) if no match.
// This is O(k) where k is the length of the longest match.
func (t *FastTrie) LongestMatch(text []byte) (int, int) {
	node := t.root
	bestLen := 0
	bestID := -1

	for i, b := range text {
		child := node.children[b]
		if child == nil {
			break
		}
		node = child
		if node.isToken {
			bestLen = i + 1
			bestID = node.tokenID
		}
	}

	return bestLen, bestID
}

// Encoder provides fast greedy BPE encoding using FastTrie.
type Encoder struct {
	vocab *Vocabulary
	trie  *FastTrie
}

// NewEncoder creates an encoder for the given vocabulary.
func NewEncoder(vocab *Vocabulary) *Encoder {
	trie := NewFastTrie()
	for token, id := range vocab.AllTokens() {
		trie.Insert([]byte(token), id)
	}
	return &Encoder{
		vocab: vocab,
		trie:  trie,
	}
}

// Encode tokenizes text using greedy longest-match with O(n) complexity.
func (e *Encoder) Encode(text []byte) []int {
	if len(text) == 0 {
		return nil
	}

	result := make([]int, 0, len(text)/4+1)
	pos := 0

	for pos < len(text) {
		matchLen, tokenID := e.trie.LongestMatch(text[pos:])

		if matchLen == 0 {
			// No match - use single byte token
			if id, ok := e.vocab.GetID(text[pos : pos+1]); ok {
				result = append(result, id)
			} else {
				result = append(result, int(text[pos]))
			}
			pos++
		} else {
			result = append(result, tokenID)
			pos += matchLen
		}
	}

	return result
}

// Decode converts token IDs back to bytes.
func (e *Encoder) Decode(ids []int) []byte {
	return e.vocab.Decode(ids)
}

// Vocabulary returns the underlying vocabulary.
func (e *Encoder) Vocabulary() *Vocabulary {
	return e.vocab
}
