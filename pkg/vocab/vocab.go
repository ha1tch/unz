// Package vocab provides pre-trained BPE vocabularies for compression.
package vocab

import (
	"github.com/ha1tch/unz/pkg/bpe"
)

// Language represents a programming language or text type.
type Language int

const (
	LangText       Language = iota // Natural language text (default)
	LangGo                         // Go source code
	LangPython                     // Python source code
	LangJavaScript                 // JavaScript/TypeScript source code
)

func (l Language) String() string {
	switch l {
	case LangGo:
		return "Go"
	case LangPython:
		return "Python"
	case LangJavaScript:
		return "JavaScript"
	default:
		return "Text"
	}
}

var (
	defaultVocab *bpe.Vocabulary
	goVocab      *bpe.Vocabulary
	pythonVocab  *bpe.Vocabulary
	jsVocab      *bpe.Vocabulary
)

// Default returns the default BPE vocabulary for natural language text.
func Default() *bpe.Vocabulary {
	if defaultVocab == nil {
		defaultVocab = bpe.NewVocabulary(defaultTokens)
	}
	return defaultVocab
}

// ForLanguage returns the BPE vocabulary for the specified language.
func ForLanguage(lang Language) *bpe.Vocabulary {
	switch lang {
	case LangGo:
		if goVocab == nil {
			goVocab = bpe.NewVocabulary(GoTokens)
		}
		return goVocab
	case LangPython:
		if pythonVocab == nil {
			pythonVocab = bpe.NewVocabulary(PythonTokens)
		}
		return pythonVocab
	case LangJavaScript:
		if jsVocab == nil {
			jsVocab = bpe.NewVocabulary(JSTokens)
		}
		return jsVocab
	default:
		return Default()
	}
}

// Size returns the number of tokens in the default vocabulary.
func Size() int {
	return len(defaultTokens)
}

// SizeForLanguage returns the number of tokens in a language vocabulary.
func SizeForLanguage(lang Language) int {
	switch lang {
	case LangGo:
		return len(GoTokens)
	case LangPython:
		return len(PythonTokens)
	case LangJavaScript:
		return len(JSTokens)
	default:
		return len(defaultTokens)
	}
}
