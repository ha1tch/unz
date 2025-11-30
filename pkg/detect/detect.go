// Package detect provides automatic detection of data types for
// selecting optimal compression pipelines.
package detect

import (
	"bytes"
	"math"
)

// Type represents the detected type of input data.
type Type int

const (
	TypeText       Type = iota // Natural language prose
	TypeCode                   // Source code / structured text
	TypeBinary                 // General binary
	TypeRepetitive             // Highly repetitive data
	TypeLowEntropy             // Low entropy (restricted byte range)
	TypeRandom                 // High entropy, incompressible
)

func (t Type) String() string {
	switch t {
	case TypeText:
		return "text"
	case TypeCode:
		return "code"
	case TypeBinary:
		return "binary"
	case TypeRepetitive:
		return "repetitive"
	case TypeLowEntropy:
		return "low-entropy"
	case TypeRandom:
		return "random"
	default:
		return "unknown"
	}
}

// CodeLang represents a detected programming language.
type CodeLang int

const (
	CodeLangUnknown CodeLang = iota
	CodeLangGo
	CodeLangPython
	CodeLangJavaScript
	CodeLangJava
	CodeLangC
	CodeLangCPP
	CodeLangCSharp
	CodeLangRuby
	CodeLangRust
	CodeLangPHP
	CodeLangSwift
	CodeLangKotlin
)

func (l CodeLang) String() string {
	names := []string{
		"Unknown", "Go", "Python", "JavaScript", "Java", "C", "C++",
		"C#", "Ruby", "Rust", "PHP", "Swift", "Kotlin",
	}
	if int(l) < len(names) {
		return names[l]
	}
	return "Unknown"
}

// DataFormat represents a detected structured data format.
type DataFormat int

const (
	DataFormatNone DataFormat = iota
	DataFormatJSON
	DataFormatXML
	DataFormatYAML
	DataFormatCSV
	DataFormatTOML
	DataFormatINI
)

func (d DataFormat) String() string {
	names := []string{"None", "JSON", "XML", "YAML", "CSV", "TOML", "INI"}
	if int(d) < len(names) {
		return names[d]
	}
	return "Unknown"
}

// MarkupLang represents a detected markup language.
type MarkupLang int

const (
	MarkupNone MarkupLang = iota
	MarkupHTML
	MarkupMarkdown
	MarkupLaTeX
	MarkupRTF
	MarkupReST
	MarkupAsciiDoc
	MarkupOrg
)

func (m MarkupLang) String() string {
	names := []string{"None", "HTML", "Markdown", "LaTeX", "RTF", "reST", "AsciiDoc", "Org"}
	if int(m) < len(names) {
		return names[m]
	}
	return "Unknown"
}

// NatLang represents a detected natural/human language.
type NatLang int

const (
	NatLangUnknown NatLang = iota
	NatLangEnglish
	NatLangSpanish
	NatLangFrench
	NatLangPortuguese
	NatLangGerman
	NatLangItalian
	NatLangDutch
	NatLangChinese
	NatLangArabic
	NatLangHindi
	NatLangIndonesian
	NatLangBengali
	NatLangRussian
	NatLangJapanese
)

func (n NatLang) String() string {
	names := []string{
		"Unknown", "English", "Spanish", "French", "Portuguese", "German",
		"Italian", "Dutch", "Chinese", "Arabic", "Hindi", "Indonesian",
		"Bengali", "Russian", "Japanese",
	}
	if int(n) < len(names) {
		return names[n]
	}
	return "Unknown"
}

// Profile contains statistics about input data.
type Profile struct {
	Type           Type
	Language       CodeLang   // Programming language (if TypeCode)
	DataFmt        DataFormat // Structured data format
	Markup         MarkupLang // Markup language
	NatLang        NatLang    // Natural/human language
	Entropy        float64    // bits per byte (0-8)
	ASCIIRatio     float64    // fraction of printable ASCII
	UniqueBytes    int        // number of distinct byte values
	RepetitionRate float64    // estimated repetition (0-1)
	CodeScore      float64    // likelihood of being source code (0-1)
}

// Detect analyzes data and returns its profile.
// Uses first 8KB for analysis if data is larger.
func Detect(data []byte) Profile {
	if len(data) == 0 {
		return Profile{Type: TypeRandom}
	}

	// Sample size (use first 8KB or all if smaller)
	sampleSize := len(data)
	if sampleSize > 8192 {
		sampleSize = 8192
	}
	sample := data[:sampleSize]

	// Compute byte frequency histogram
	var freq [256]int
	for _, b := range sample {
		freq[b]++
	}

	// Count unique bytes
	uniqueBytes := 0
	for _, f := range freq {
		if f > 0 {
			uniqueBytes++
		}
	}

	// Compute entropy
	entropy := 0.0
	n := float64(len(sample))
	for _, f := range freq {
		if f > 0 {
			p := float64(f) / n
			entropy -= p * math.Log2(p)
		}
	}

	// Count ASCII printable characters (0x20-0x7E, plus \t, \n, \r)
	asciiCount := 0
	for _, b := range sample {
		if (b >= 0x20 && b <= 0x7E) || b == '\t' || b == '\n' || b == '\r' {
			asciiCount++
		}
	}
	asciiRatio := float64(asciiCount) / float64(len(sample))

	// Estimate repetition rate
	repetitionRate := estimateRepetition(sample)

	// Compute code score
	codeScore := computeCodeScore(sample, freq[:])

	profile := Profile{
		Entropy:        entropy,
		ASCIIRatio:     asciiRatio,
		UniqueBytes:    uniqueBytes,
		RepetitionRate: repetitionRate,
		CodeScore:      codeScore,
	}

	// First check for structured data formats (JSON, XML, etc.)
	profile.DataFmt = detectDataFormat(sample)

	// Check for markup languages
	profile.Markup = detectMarkup(sample)

	// Detect natural language (for text content)
	profile.NatLang = detectNatLang(sample)

	// Classify data type
	switch {
	case profile.DataFmt != DataFormatNone:
		// Structured data is a type of code
		profile.Type = TypeCode
	case profile.Markup != MarkupNone:
		// Markup is a type of text (unless it's HTML with scripts)
		if profile.Markup == MarkupHTML && codeScore >= 0.4 {
			profile.Type = TypeCode
			profile.Language = CodeLangJavaScript // likely has embedded JS
		} else {
			profile.Type = TypeText
		}
	case asciiRatio > 0.85 && codeScore >= 0.4:
		profile.Type = TypeCode
		profile.Language = detectLanguage(sample)
	case asciiRatio > 0.85:
		profile.Type = TypeText
	case repetitionRate > 0.3:
		profile.Type = TypeRepetitive
	case entropy < 5.0:
		profile.Type = TypeLowEntropy
	case entropy > 7.5 && uniqueBytes > 250:
		profile.Type = TypeRandom
	default:
		profile.Type = TypeBinary
	}

	return profile
}

// estimateRepetition estimates how repetitive the data is
func estimateRepetition(data []byte) float64 {
	if len(data) < 8 {
		return 0
	}

	// Count 4-byte sequences that repeat
	seen := make(map[uint32]int)
	repeats := 0
	total := 0

	for i := 0; i <= len(data)-4; i += 2 {
		hash := uint32(data[i]) | uint32(data[i+1])<<8 |
			uint32(data[i+2])<<16 | uint32(data[i+3])<<24

		if seen[hash] > 0 {
			repeats++
		}
		seen[hash]++
		total++
	}

	if total == 0 {
		return 0
	}
	return float64(repeats) / float64(total)
}

// computeCodeScore estimates likelihood that data is source code
func computeCodeScore(data []byte, freq []int) float64 {
	n := float64(len(data))
	if n == 0 {
		return 0
	}

	score := 0.0

	// Code indicators:
	// 1. High frequency of brackets, braces, semicolons
	brackets := freq['{'] + freq['}'] + freq['['] + freq[']'] + freq['('] + freq[')']
	bracketRatio := float64(brackets) / n
	if bracketRatio > 0.02 {
		score += 0.3
	}

	// 2. Semicolons or colons (statements, JSON)
	punctuation := freq[';'] + freq[':']
	punctRatio := float64(punctuation) / n
	if punctRatio > 0.01 {
		score += 0.2
	}

	// 3. Quotes (strings in code)
	quotes := freq['"'] + freq['\'']
	quoteRatio := float64(quotes) / n
	if quoteRatio > 0.02 {
		score += 0.1
	}

	// 4. Indentation (tabs or leading spaces)
	tabs := freq['\t']
	tabRatio := float64(tabs) / n
	if tabRatio > 0.02 {
		score += 0.2
	}

	// 5. Low space ratio (code typically 0.05-0.15 vs prose 0.15-0.20)
	spaces := freq[' ']
	spaceRatio := float64(spaces) / n
	if spaceRatio < 0.12 {
		score += 0.1
	}

	// 6. Operators common in code
	operators := freq['='] + freq['+'] + freq['-'] + freq['*'] + freq['/'] + freq['<'] + freq['>']
	opRatio := float64(operators) / n
	if opRatio > 0.01 {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// detectLanguage identifies the programming language of source code.
func detectLanguage(data []byte) CodeLang {
	goScore := 0
	pyScore := 0
	jsScore := 0

	// Go indicators
	if bytes.Contains(data, []byte("package ")) {
		goScore += 3
	}
	if bytes.Contains(data, []byte("func ")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte(":= ")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("if err != nil")) {
		goScore += 3
	}
	if bytes.Contains(data, []byte("import (")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("defer ")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("go func")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("chan ")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("interface{")) {
		goScore += 2
	}
	if bytes.Contains(data, []byte("struct {")) {
		goScore += 2
	}

	// Python indicators
	if bytes.Contains(data, []byte("def ")) {
		pyScore += 2
	}
	if bytes.Contains(data, []byte("class ")) {
		pyScore += 1 // Also in other languages
	}
	if bytes.Contains(data, []byte("import ")) {
		pyScore += 1 // Also in JS/Go
	}
	if bytes.Contains(data, []byte("from ")) && bytes.Contains(data, []byte(" import ")) {
		pyScore += 3
	}
	if bytes.Contains(data, []byte("self.")) {
		pyScore += 3
	}
	if bytes.Contains(data, []byte("__init__")) {
		pyScore += 3
	}
	if bytes.Contains(data, []byte("__name__")) {
		pyScore += 3
	}
	if bytes.Contains(data, []byte("elif ")) {
		pyScore += 3
	}
	if bytes.Contains(data, []byte("True")) || bytes.Contains(data, []byte("False")) {
		pyScore += 1
	}
	if bytes.Contains(data, []byte("None")) {
		pyScore += 1
	}
	if bytes.Contains(data, []byte("async def")) {
		pyScore += 2
	}
	if bytes.Contains(data, []byte("await ")) {
		pyScore += 1 // Also in JS
	}
	// Python uses significant indentation - check for 4-space indents
	if bytes.Count(data, []byte("    ")) > bytes.Count(data, []byte("\t")) {
		pyScore += 1
	}

	// JavaScript/TypeScript indicators
	if bytes.Contains(data, []byte("const ")) {
		jsScore += 2
	}
	if bytes.Contains(data, []byte("let ")) {
		jsScore += 2
	}
	if bytes.Contains(data, []byte("var ")) {
		jsScore += 1
	}
	if bytes.Contains(data, []byte("function ")) {
		jsScore += 1 // Could be PHP too
	}
	if bytes.Contains(data, []byte("=> ")) {
		jsScore += 3
	}
	if bytes.Contains(data, []byte("async ")) {
		jsScore += 1 // Also in Python
	}
	if bytes.Contains(data, []byte("export ")) {
		jsScore += 2
	}
	if bytes.Contains(data, []byte("require(")) {
		jsScore += 3
	}
	if bytes.Contains(data, []byte("module.exports")) {
		jsScore += 3
	}
	if bytes.Contains(data, []byte("console.log")) {
		jsScore += 2
	}
	if bytes.Contains(data, []byte("null")) {
		jsScore += 1
	}
	if bytes.Contains(data, []byte("undefined")) {
		jsScore += 2
	}
	if bytes.Contains(data, []byte("interface ")) {
		jsScore += 1 // TypeScript
	}
	if bytes.Contains(data, []byte(": string")) || bytes.Contains(data, []byte(": number")) {
		jsScore += 2 // TypeScript
	}
	if bytes.Contains(data, []byte("useState")) || bytes.Contains(data, []byte("useEffect")) {
		jsScore += 3 // React
	}

	// Java indicators
	javaScore := 0
	if bytes.Contains(data, []byte("public class ")) {
		javaScore += 4
	}
	if bytes.Contains(data, []byte("private ")) {
		javaScore += 1
	}
	if bytes.Contains(data, []byte("public static void main")) {
		javaScore += 5
	}
	if bytes.Contains(data, []byte("System.out.println")) {
		javaScore += 4
	}
	if bytes.Contains(data, []byte("import java.")) {
		javaScore += 4
	}
	if bytes.Contains(data, []byte("@Override")) {
		javaScore += 3
	}
	if bytes.Contains(data, []byte("throws ")) {
		javaScore += 2
	}
	if bytes.Contains(data, []byte("extends ")) {
		javaScore += 1
	}
	if bytes.Contains(data, []byte("implements ")) {
		javaScore += 2
	}

	// C indicators
	cScore := 0
	if bytes.Contains(data, []byte("#include <")) {
		cScore += 4
	}
	if bytes.Contains(data, []byte("#include \"")) {
		cScore += 3
	}
	if bytes.Contains(data, []byte("#define ")) {
		cScore += 3
	}
	if bytes.Contains(data, []byte("int main(")) {
		cScore += 4
	}
	if bytes.Contains(data, []byte("printf(")) {
		cScore += 3
	}
	if bytes.Contains(data, []byte("malloc(")) || bytes.Contains(data, []byte("free(")) {
		cScore += 3
	}
	if bytes.Contains(data, []byte("sizeof(")) {
		cScore += 2
	}
	if bytes.Contains(data, []byte("typedef ")) {
		cScore += 2
	}
	if bytes.Contains(data, []byte("->")) {
		cScore += 1
	}

	// C++ indicators
	cppScore := cScore // Inherits from C
	if bytes.Contains(data, []byte("std::")) {
		cppScore += 4
	}
	if bytes.Contains(data, []byte("cout <<")) || bytes.Contains(data, []byte("cin >>")) {
		cppScore += 4
	}
	if bytes.Contains(data, []byte("namespace ")) {
		cppScore += 3
	}
	if bytes.Contains(data, []byte("template<")) || bytes.Contains(data, []byte("template <")) {
		cppScore += 4
	}
	if bytes.Contains(data, []byte("class ")) && bytes.Contains(data, []byte("public:")) {
		cppScore += 3
	}
	if bytes.Contains(data, []byte("virtual ")) {
		cppScore += 2
	}
	if bytes.Contains(data, []byte("nullptr")) {
		cppScore += 3
	}
	if bytes.Contains(data, []byte("new ")) && bytes.Contains(data, []byte("delete ")) {
		cppScore += 2
	}

	// C# indicators
	csScore := 0
	if bytes.Contains(data, []byte("using System")) {
		csScore += 4
	}
	if bytes.Contains(data, []byte("namespace ")) && bytes.Contains(data, []byte("class ")) {
		csScore += 2
	}
	if bytes.Contains(data, []byte("Console.WriteLine")) {
		csScore += 4
	}
	if bytes.Contains(data, []byte("public async Task")) {
		csScore += 4
	}
	if bytes.Contains(data, []byte("[Attribute]")) || bytes.Contains(data, []byte("[SerializeField]")) {
		csScore += 3
	}
	if bytes.Contains(data, []byte("get;")) || bytes.Contains(data, []byte("set;")) {
		csScore += 3
	}
	if bytes.Contains(data, []byte("var ")) && bytes.Contains(data, []byte("new ")) {
		csScore += 1
	}
	if bytes.Contains(data, []byte("=> ")) && bytes.Contains(data, []byte("public ")) {
		csScore += 2
	}

	// Ruby indicators
	rubyScore := 0
	if bytes.Contains(data, []byte("def ")) && bytes.Contains(data, []byte("end")) {
		rubyScore += 3
	}
	if bytes.Contains(data, []byte("require '")) || bytes.Contains(data, []byte("require \"")) {
		rubyScore += 3
	}
	if bytes.Contains(data, []byte("puts ")) {
		rubyScore += 3
	}
	if bytes.Contains(data, []byte("attr_accessor")) || bytes.Contains(data, []byte("attr_reader")) {
		rubyScore += 4
	}
	if bytes.Contains(data, []byte("do |")) {
		rubyScore += 3
	}
	if bytes.Contains(data, []byte(".each ")) || bytes.Contains(data, []byte(".map ")) {
		rubyScore += 2
	}
	if bytes.Contains(data, []byte("@")) && bytes.Contains(data, []byte("def ")) {
		rubyScore += 2 // Instance variables
	}
	if bytes.Contains(data, []byte("unless ")) || bytes.Contains(data, []byte("elsif ")) {
		rubyScore += 2
	}
	if bytes.Contains(data, []byte("module ")) {
		rubyScore += 2
	}

	// Rust indicators
	rustScore := 0
	if bytes.Contains(data, []byte("fn ")) && bytes.Contains(data, []byte("let ")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("fn main()")) {
		rustScore += 4
	}
	if bytes.Contains(data, []byte("println!(")) || bytes.Contains(data, []byte("print!(")) {
		rustScore += 4
	}
	if bytes.Contains(data, []byte("use std::")) {
		rustScore += 4
	}
	if bytes.Contains(data, []byte("mut ")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("impl ")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("pub fn ")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("match ")) && bytes.Contains(data, []byte("=>")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("Option<")) || bytes.Contains(data, []byte("Result<")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("&self")) || bytes.Contains(data, []byte("&mut self")) {
		rustScore += 3
	}
	if bytes.Contains(data, []byte("unwrap()")) {
		rustScore += 2
	}

	// PHP indicators
	phpScore := 0
	if bytes.Contains(data, []byte("<?php")) {
		phpScore += 5
	}
	if bytes.Contains(data, []byte("echo ")) {
		phpScore += 2
	}
	if bytes.Contains(data, []byte("$")) && bytes.Contains(data, []byte("function ")) {
		phpScore += 3
	}
	if bytes.Contains(data, []byte("->")) && bytes.Contains(data, []byte("$this")) {
		phpScore += 4
	}
	if bytes.Contains(data, []byte("namespace ")) && bytes.Contains(data, []byte("use ")) {
		phpScore += 2
	}
	if bytes.Contains(data, []byte("array(")) || bytes.Contains(data, []byte("=>")) {
		phpScore += 1
	}

	// Swift indicators
	swiftScore := 0
	if bytes.Contains(data, []byte("import Foundation")) || bytes.Contains(data, []byte("import UIKit")) {
		swiftScore += 5
	}
	if bytes.Contains(data, []byte("func ")) && bytes.Contains(data, []byte("->")) {
		swiftScore += 3
	}
	if bytes.Contains(data, []byte("var ")) && bytes.Contains(data, []byte(": ")) {
		swiftScore += 2
	}
	if bytes.Contains(data, []byte("let ")) && bytes.Contains(data, []byte(": ")) {
		swiftScore += 2
	}
	if bytes.Contains(data, []byte("guard ")) || bytes.Contains(data, []byte("if let ")) {
		swiftScore += 4
	}
	if bytes.Contains(data, []byte("print(")) && !bytes.Contains(data, []byte("println")) {
		swiftScore += 2
	}
	if bytes.Contains(data, []byte("@IBOutlet")) || bytes.Contains(data, []byte("@IBAction")) {
		swiftScore += 4
	}
	if bytes.Contains(data, []byte("struct ")) && bytes.Contains(data, []byte(": ")) {
		swiftScore += 2
	}

	// Kotlin indicators
	kotlinScore := 0
	if bytes.Contains(data, []byte("fun ")) && bytes.Contains(data, []byte(": ")) {
		kotlinScore += 3
	}
	if bytes.Contains(data, []byte("fun main(")) {
		kotlinScore += 4
	}
	if bytes.Contains(data, []byte("println(")) {
		kotlinScore += 2
	}
	if bytes.Contains(data, []byte("val ")) || bytes.Contains(data, []byte("var ")) {
		kotlinScore += 1
	}
	if bytes.Contains(data, []byte("?.")) || bytes.Contains(data, []byte("!!")) {
		kotlinScore += 3
	}
	if bytes.Contains(data, []byte("data class ")) {
		kotlinScore += 4
	}
	if bytes.Contains(data, []byte("suspend fun ")) {
		kotlinScore += 4
	}
	if bytes.Contains(data, []byte("companion object")) {
		kotlinScore += 4
	}
	if bytes.Contains(data, []byte("import kotlin")) || bytes.Contains(data, []byte("import android")) {
		kotlinScore += 4
	}

	// Determine winner
	scores := []struct {
		score int
		lang  CodeLang
	}{
		{goScore, CodeLangGo},
		{pyScore, CodeLangPython},
		{jsScore, CodeLangJavaScript},
		{javaScore, CodeLangJava},
		{cScore, CodeLangC},
		{cppScore, CodeLangCPP},
		{csScore, CodeLangCSharp},
		{rubyScore, CodeLangRuby},
		{rustScore, CodeLangRust},
		{phpScore, CodeLangPHP},
		{swiftScore, CodeLangSwift},
		{kotlinScore, CodeLangKotlin},
	}

	maxScore := 0
	lang := CodeLangUnknown

	for _, s := range scores {
		if s.score > maxScore {
			maxScore = s.score
			lang = s.lang
		}
	}

	// Minimum threshold to claim a language
	if maxScore < 3 {
		return CodeLangUnknown
	}

	return lang
}

// detectDataFormat identifies structured data formats.
func detectDataFormat(data []byte) DataFormat {
	if len(data) == 0 {
		return DataFormatNone
	}

	// Trim whitespace for detection
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return DataFormatNone
	}

	// JSON: starts with { or [
	if (trimmed[0] == '{' || trimmed[0] == '[') &&
		(bytes.Contains(data, []byte("\":")) || bytes.Contains(data, []byte("\": "))) {
		return DataFormatJSON
	}

	// XML: starts with < and contains <?xml or has tag structure
	if trimmed[0] == '<' {
		if bytes.Contains(data, []byte("<?xml")) {
			return DataFormatXML
		}
		if bytes.Contains(data, []byte("</")) && bytes.Contains(data, []byte(">")) {
			// Could be HTML or XML - check for HTML markers
			if !bytes.Contains(bytes.ToLower(data), []byte("<html")) &&
				!bytes.Contains(bytes.ToLower(data), []byte("<!doctype html")) {
				return DataFormatXML
			}
		}
	}

	// YAML: key: value patterns without JSON braces
	if bytes.Contains(data, []byte(": ")) &&
		!bytes.Contains(data, []byte("{")) &&
		(bytes.HasPrefix(trimmed, []byte("---")) ||
			bytes.Contains(data, []byte("\n  ")) ||
			bytes.Contains(data, []byte("\n- "))) {
		return DataFormatYAML
	}

	// CSV: comma-separated with consistent column count
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) >= 2 {
		commas1 := bytes.Count(lines[0], []byte(","))
		commas2 := bytes.Count(lines[1], []byte(","))
		if commas1 > 0 && commas1 == commas2 && !bytes.Contains(lines[0], []byte("{")) {
			return DataFormatCSV
		}
	}

	// TOML: [section] headers and key = value
	if bytes.Contains(data, []byte("[")) && bytes.Contains(data, []byte("]")) &&
		bytes.Contains(data, []byte(" = ")) {
		// Check for TOML-style section headers
		if bytes.Contains(data, []byte("\n[")) || bytes.HasPrefix(trimmed, []byte("[")) {
			return DataFormatTOML
		}
	}

	// INI: similar to TOML but uses = without spaces typically
	if bytes.Contains(data, []byte("[")) && bytes.Contains(data, []byte("=")) {
		if bytes.Contains(data, []byte("\n[")) || bytes.HasPrefix(trimmed, []byte("[")) {
			return DataFormatINI
		}
	}

	return DataFormatNone
}

// detectMarkup identifies markup languages.
func detectMarkup(data []byte) MarkupLang {
	if len(data) == 0 {
		return MarkupNone
	}

	lower := bytes.ToLower(data)

	// HTML: doctype or html/head/body tags
	if bytes.Contains(lower, []byte("<!doctype html")) ||
		bytes.Contains(lower, []byte("<html")) ||
		(bytes.Contains(lower, []byte("<head")) && bytes.Contains(lower, []byte("<body"))) ||
		(bytes.Contains(lower, []byte("<div")) && bytes.Contains(lower, []byte("</"))) {
		return MarkupHTML
	}

	// LaTeX: \documentclass, \begin{document}, etc.
	if bytes.Contains(data, []byte("\\documentclass")) ||
		bytes.Contains(data, []byte("\\begin{document}")) ||
		bytes.Contains(data, []byte("\\usepackage")) ||
		bytes.Contains(data, []byte("\\section{")) {
		return MarkupLaTeX
	}

	// RTF: starts with {\rtf
	if bytes.HasPrefix(data, []byte("{\\rtf")) {
		return MarkupRTF
	}

	// Org-mode: starts with * or has #+TITLE
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("* ")) ||
		bytes.Contains(data, []byte("#+TITLE")) ||
		bytes.Contains(data, []byte("#+BEGIN_SRC")) {
		return MarkupOrg
	}

	// reStructuredText: underlined headers with === or ---
	lines := bytes.Split(data, []byte("\n"))
	for i := 1; i < len(lines); i++ {
		line := bytes.TrimSpace(lines[i])
		if len(line) > 3 {
			allSame := true
			ch := line[0]
			if ch == '=' || ch == '-' || ch == '~' || ch == '^' {
				for _, b := range line {
					if b != ch {
						allSame = false
						break
					}
				}
				if allSame && len(lines[i-1]) > 0 {
					// Check for .. directive syntax too
					if bytes.Contains(data, []byte(".. ")) {
						return MarkupReST
					}
				}
			}
		}
	}

	// AsciiDoc: = Title or == Section
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("= ")) ||
		bytes.Contains(data, []byte("\n== ")) ||
		bytes.Contains(data, []byte(":toc:")) ||
		bytes.Contains(data, []byte("[source,")) {
		return MarkupAsciiDoc
	}

	// Markdown: # headers, ``` code blocks, [links](url), **bold**
	mdScore := 0
	if bytes.Contains(data, []byte("# ")) {
		mdScore += 2
	}
	if bytes.Contains(data, []byte("## ")) {
		mdScore += 2
	}
	if bytes.Contains(data, []byte("```")) {
		mdScore += 3
	}
	if bytes.Contains(data, []byte("](")) && bytes.Contains(data, []byte("[")) {
		mdScore += 2
	}
	if bytes.Contains(data, []byte("**")) || bytes.Contains(data, []byte("__")) {
		mdScore += 1
	}
	if bytes.Contains(data, []byte("- ")) || bytes.Contains(data, []byte("* ")) {
		mdScore += 1
	}
	if mdScore >= 3 {
		return MarkupMarkdown
	}

	return MarkupNone
}

// detectNatLang identifies the natural/human language of text.
func detectNatLang(data []byte) NatLang {
	if len(data) == 0 {
		return NatLangUnknown
	}

	// Check for non-Latin scripts first (by Unicode range)
	var hasArabic, hasChinese, hasJapanese, hasHindi, hasBengali, hasRussian bool

	for i := 0; i < len(data); {
		r, size := decodeRune(data[i:])
		if size == 0 {
			i++
			continue
		}

		switch {
		case r >= 0x0600 && r <= 0x06FF: // Arabic
			hasArabic = true
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs (Chinese)
			hasChinese = true
		case r >= 0x3040 && r <= 0x30FF: // Hiragana + Katakana (Japanese)
			hasJapanese = true
		case r >= 0x0900 && r <= 0x097F: // Devanagari (Hindi)
			hasHindi = true
		case r >= 0x0980 && r <= 0x09FF: // Bengali
			hasBengali = true
		case r >= 0x0400 && r <= 0x04FF: // Cyrillic (Russian)
			hasRussian = true
		}
		i += size
	}

	// Non-Latin scripts take priority
	if hasArabic {
		return NatLangArabic
	}
	if hasJapanese {
		return NatLangJapanese
	}
	if hasChinese {
		return NatLangChinese
	}
	if hasHindi {
		return NatLangHindi
	}
	if hasBengali {
		return NatLangBengali
	}
	if hasRussian {
		return NatLangRussian
	}

	// For Latin scripts, use common word detection
	lower := bytes.ToLower(data)

	// Spanish indicators
	esScore := 0
	for _, word := range [][]byte{
		[]byte(" el "), []byte(" la "), []byte(" los "), []byte(" las "),
		[]byte(" de "), []byte(" en "), []byte(" que "), []byte(" es "),
		[]byte(" un "), []byte(" una "), []byte(" para "), []byte(" con "),
		[]byte(" por "), []byte("ción"), []byte("ñ"),
	} {
		if bytes.Contains(lower, word) {
			esScore++
		}
	}

	// French indicators
	frScore := 0
	for _, word := range [][]byte{
		[]byte(" le "), []byte(" la "), []byte(" les "), []byte(" de "),
		[]byte(" et "), []byte(" est "), []byte(" un "), []byte(" une "),
		[]byte(" que "), []byte(" pour "), []byte(" avec "), []byte(" dans "),
		[]byte("ç"), []byte("œ"), []byte("ê"), []byte("è"),
	} {
		if bytes.Contains(lower, word) {
			frScore++
		}
	}

	// Portuguese indicators
	ptScore := 0
	for _, word := range [][]byte{
		[]byte(" o "), []byte(" a "), []byte(" os "), []byte(" as "),
		[]byte(" de "), []byte(" em "), []byte(" que "), []byte(" é "),
		[]byte(" um "), []byte(" uma "), []byte(" para "), []byte(" com "),
		[]byte(" não "), []byte("ção"), []byte("ã"),
	} {
		if bytes.Contains(lower, word) {
			ptScore++
		}
	}

	// German indicators
	deScore := 0
	for _, word := range [][]byte{
		[]byte(" der "), []byte(" die "), []byte(" das "), []byte(" und "),
		[]byte(" ist "), []byte(" ein "), []byte(" eine "), []byte(" für "),
		[]byte(" mit "), []byte(" auf "), []byte(" nicht "), []byte(" sich "),
		[]byte("ß"), []byte("ü"), []byte("ö"), []byte("ä"),
	} {
		if bytes.Contains(lower, word) {
			deScore++
		}
	}

	// Italian indicators
	itScore := 0
	for _, word := range [][]byte{
		[]byte(" il "), []byte(" la "), []byte(" i "), []byte(" le "),
		[]byte(" di "), []byte(" che "), []byte(" è "), []byte(" un "),
		[]byte(" una "), []byte(" per "), []byte(" con "), []byte(" non "),
		[]byte(" sono "), []byte(" della "),
	} {
		if bytes.Contains(lower, word) {
			itScore++
		}
	}

	// Dutch indicators
	nlScore := 0
	for _, word := range [][]byte{
		[]byte(" de "), []byte(" het "), []byte(" een "), []byte(" van "),
		[]byte(" en "), []byte(" is "), []byte(" op "), []byte(" te "),
		[]byte(" dat "), []byte(" niet "), []byte(" met "), []byte(" voor "),
		[]byte("ij"), []byte("oe"),
	} {
		if bytes.Contains(lower, word) {
			nlScore++
		}
	}

	// Indonesian/Malay indicators
	idScore := 0
	for _, word := range [][]byte{
		[]byte(" yang "), []byte(" dan "), []byte(" di "), []byte(" ini "),
		[]byte(" itu "), []byte(" dengan "), []byte(" untuk "), []byte(" dari "),
		[]byte(" pada "), []byte(" adalah "), []byte(" tidak "), []byte(" ke "),
		[]byte("kan "), []byte("nya "),
	} {
		if bytes.Contains(lower, word) {
			idScore++
		}
	}

	// English indicators (baseline)
	enScore := 0
	for _, word := range [][]byte{
		[]byte(" the "), []byte(" and "), []byte(" is "), []byte(" to "),
		[]byte(" of "), []byte(" a "), []byte(" in "), []byte(" that "),
		[]byte(" it "), []byte(" for "), []byte(" with "), []byte(" as "),
		[]byte(" was "), []byte(" are "), []byte(" be "), []byte(" have "),
	} {
		if bytes.Contains(lower, word) {
			enScore++
		}
	}

	// Find the highest scoring language
	scores := []struct {
		score int
		lang  NatLang
	}{
		{enScore, NatLangEnglish},
		{esScore, NatLangSpanish},
		{frScore, NatLangFrench},
		{ptScore, NatLangPortuguese},
		{deScore, NatLangGerman},
		{itScore, NatLangItalian},
		{nlScore, NatLangDutch},
		{idScore, NatLangIndonesian},
	}

	maxScore := 0
	lang := NatLangUnknown

	for _, s := range scores {
		if s.score > maxScore {
			maxScore = s.score
			lang = s.lang
		}
	}

	// Require minimum confidence
	if maxScore < 3 {
		return NatLangUnknown
	}

	return lang
}

// decodeRune decodes the first UTF-8 rune from data.
// Returns the rune and its byte length, or (0, 0) for invalid encoding.
func decodeRune(data []byte) (rune, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b0 := data[0]

	// ASCII
	if b0 < 0x80 {
		return rune(b0), 1
	}

	// 2-byte sequence
	if b0 >= 0xC0 && b0 < 0xE0 && len(data) >= 2 {
		return rune(b0&0x1F)<<6 | rune(data[1]&0x3F), 2
	}

	// 3-byte sequence
	if b0 >= 0xE0 && b0 < 0xF0 && len(data) >= 3 {
		return rune(b0&0x0F)<<12 | rune(data[1]&0x3F)<<6 | rune(data[2]&0x3F), 3
	}

	// 4-byte sequence
	if b0 >= 0xF0 && b0 < 0xF8 && len(data) >= 4 {
		return rune(b0&0x07)<<18 | rune(data[1]&0x3F)<<12 | rune(data[2]&0x3F)<<6 | rune(data[3]&0x3F), 4
	}

	return 0, 1 // Invalid, skip one byte
}
