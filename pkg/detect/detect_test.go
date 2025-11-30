package detect

import (
	"bytes"
	"strings"
	"testing"
)

func TestDetectText(t *testing.T) {
	text := []byte("The quick brown fox jumps over the lazy dog. This is a sample of natural language text that should be detected as prose.")

	profile := Detect(text)

	if profile.Type != TypeText {
		t.Errorf("type: got %v, want TypeText", profile.Type)
	}

	if profile.ASCIIRatio < 0.85 {
		t.Errorf("ASCII ratio too low: %f", profile.ASCIIRatio)
	}

	if profile.CodeScore > 0.4 {
		t.Errorf("code score too high for prose: %f", profile.CodeScore)
	}
}

func TestDetectCode(t *testing.T) {
	code := []byte(`func main() {
	fmt.Println("Hello, World!")
	for i := 0; i < 10; i++ {
		result := compute(i)
		fmt.Printf("%d: %d\n", i, result)
	}
}`)

	profile := Detect(code)

	if profile.Type != TypeCode {
		t.Errorf("type: got %v, want TypeCode", profile.Type)
	}

	if profile.CodeScore < 0.4 {
		t.Errorf("code score too low: %f", profile.CodeScore)
	}
}

func TestDetectJSON(t *testing.T) {
	json := []byte(`{
	"name": "test",
	"value": 123,
	"items": ["a", "b", "c"],
	"nested": {
		"foo": "bar"
	}
}`)

	profile := Detect(json)

	if profile.Type != TypeCode {
		t.Errorf("type: got %v, want TypeCode (JSON)", profile.Type)
	}
}

func TestDetectBinary(t *testing.T) {
	// Create binary data with varied byte values
	binary := make([]byte, 1000)
	for i := range binary {
		binary[i] = byte(i * 37 % 256)
	}

	profile := Detect(binary)

	if profile.Type == TypeText || profile.Type == TypeCode {
		t.Errorf("binary data should not be detected as text/code: got %v", profile.Type)
	}

	if profile.ASCIIRatio > 0.5 {
		t.Errorf("ASCII ratio too high for binary: %f", profile.ASCIIRatio)
	}
}

func TestDetectRepetitive(t *testing.T) {
	// Highly repetitive binary pattern
	pattern := []byte{0x12, 0x34, 0x56, 0x78}
	repetitive := bytes.Repeat(pattern, 500)

	profile := Detect(repetitive)

	if profile.Type != TypeRepetitive {
		t.Errorf("type: got %v, want TypeRepetitive", profile.Type)
	}

	if profile.RepetitionRate < 0.3 {
		t.Errorf("repetition rate too low: %f", profile.RepetitionRate)
	}
}

func TestDetectRandom(t *testing.T) {
	// High entropy random-like data (all unique bytes in sequence)
	random := make([]byte, 1000)
	for i := range random {
		// Use a pattern that produces high entropy
		random[i] = byte((i*179 + 83) % 256)
	}

	profile := Detect(random)

	if profile.Entropy < 7.0 {
		t.Errorf("entropy too low for random: %f", profile.Entropy)
	}
}

func TestDetectLowEntropy(t *testing.T) {
	// Data with very low entropy (few unique values)
	lowEntropy := make([]byte, 1000)
	for i := range lowEntropy {
		lowEntropy[i] = byte(i % 3) // Only 3 unique values
	}

	profile := Detect(lowEntropy)

	if profile.Entropy > 2.0 {
		t.Errorf("entropy too high: %f", profile.Entropy)
	}
}

func TestDetectEmpty(t *testing.T) {
	profile := Detect([]byte{})

	if profile.Type != TypeRandom {
		t.Errorf("empty data: got %v, want TypeRandom", profile.Type)
	}
}

func TestTypeString(t *testing.T) {
	testCases := []struct {
		typ  Type
		want string
	}{
		{TypeText, "text"},
		{TypeCode, "code"},
		{TypeBinary, "binary"},
		{TypeRepetitive, "repetitive"},
		{TypeLowEntropy, "low-entropy"},
		{TypeRandom, "random"},
		{Type(99), "unknown"},
	}

	for _, tc := range testCases {
		if got := tc.typ.String(); got != tc.want {
			t.Errorf("%v.String(): got %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestDetectProfile(t *testing.T) {
	text := []byte("Hello, world! This is a test.")
	profile := Detect(text)

	// Check all profile fields are populated
	if profile.Entropy <= 0 {
		t.Error("Entropy should be positive")
	}

	if profile.ASCIIRatio <= 0 || profile.ASCIIRatio > 1 {
		t.Errorf("ASCIIRatio out of range: %f", profile.ASCIIRatio)
	}

	if profile.UniqueBytes <= 0 || profile.UniqueBytes > 256 {
		t.Errorf("UniqueBytes out of range: %d", profile.UniqueBytes)
	}

	if profile.RepetitionRate < 0 || profile.RepetitionRate > 1 {
		t.Errorf("RepetitionRate out of range: %f", profile.RepetitionRate)
	}

	if profile.CodeScore < 0 || profile.CodeScore > 1 {
		t.Errorf("CodeScore out of range: %f", profile.CodeScore)
	}
}

func TestDetectSample(t *testing.T) {
	// Data larger than sample size (8KB)
	large := bytes.Repeat([]byte("Hello World! "), 1000)

	profile := Detect(large)

	// Should still work correctly
	if profile.Type != TypeText {
		t.Errorf("large text: got %v, want TypeText", profile.Type)
	}
}

func TestCodeScoreIndicators(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		minScore float64
	}{
		{"brackets", "{}[](){}[](){}", 0.3},
		{"semicolons", "a;b;c;d;e;f;g;h;", 0.2},
		{"quotes", `"a""b""c""d""e"`, 0.1},
		{"tabs", "a\tb\tc\td\te\t", 0.2},
		{"operators", "a=b+c-d*e/f<g>h", 0.1},
		{"mixed code", `func f() { x := 1; return x }`, 0.4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect([]byte(tc.input))
			if profile.CodeScore < tc.minScore {
				t.Errorf("CodeScore: got %f, want >= %f", profile.CodeScore, tc.minScore)
			}
		})
	}
}

func TestEntropyCalculation(t *testing.T) {
	testCases := []struct {
		name       string
		input      []byte
		minEntropy float64
		maxEntropy float64
	}{
		{"single byte", bytes.Repeat([]byte{0x42}, 100), 0, 0.1},
		{"two bytes equal", bytes.Repeat([]byte{0, 1}, 100), 0.9, 1.1},
		{"all bytes", makeAllBytes(), 7.9, 8.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect(tc.input)
			if profile.Entropy < tc.minEntropy || profile.Entropy > tc.maxEntropy {
				t.Errorf("Entropy: got %f, want [%f, %f]",
					profile.Entropy, tc.minEntropy, tc.maxEntropy)
			}
		})
	}
}

func TestASCIIRatio(t *testing.T) {
	// Pure ASCII
	ascii := []byte("Hello, World!")
	profile := Detect(ascii)
	if profile.ASCIIRatio < 0.99 {
		t.Errorf("pure ASCII ratio: got %f, want >= 0.99", profile.ASCIIRatio)
	}

	// Pure binary
	binary := make([]byte, 100)
	for i := range binary {
		binary[i] = byte(128 + i%128)
	}
	profile = Detect(binary)
	if profile.ASCIIRatio > 0.01 {
		t.Errorf("pure binary ratio: got %f, want <= 0.01", profile.ASCIIRatio)
	}
}

func BenchmarkDetect(b *testing.B) {
	text := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Detect(text)
	}
}

func makeAllBytes() []byte {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}

// Tests for CodeLang String()
func TestCodeLangString(t *testing.T) {
	testCases := []struct {
		lang CodeLang
		want string
	}{
		{CodeLangUnknown, "Unknown"},
		{CodeLangGo, "Go"},
		{CodeLangPython, "Python"},
		{CodeLangJavaScript, "JavaScript"},
		{CodeLangJava, "Java"},
		{CodeLangC, "C"},
		{CodeLangCPP, "C++"},
		{CodeLangCSharp, "C#"},
		{CodeLangRuby, "Ruby"},
		{CodeLangRust, "Rust"},
		{CodeLangPHP, "PHP"},
		{CodeLangSwift, "Swift"},
		{CodeLangKotlin, "Kotlin"},
		{CodeLang(99), "Unknown"},
	}

	for _, tc := range testCases {
		if got := tc.lang.String(); got != tc.want {
			t.Errorf("%d.String(): got %q, want %q", tc.lang, got, tc.want)
		}
	}
}

// Tests for DataFormat String()
func TestDataFormatString(t *testing.T) {
	testCases := []struct {
		fmt  DataFormat
		want string
	}{
		{DataFormatNone, "None"},
		{DataFormatJSON, "JSON"},
		{DataFormatXML, "XML"},
		{DataFormatYAML, "YAML"},
		{DataFormatCSV, "CSV"},
		{DataFormatTOML, "TOML"},
		{DataFormatINI, "INI"},
		{DataFormat(99), "Unknown"},
	}

	for _, tc := range testCases {
		if got := tc.fmt.String(); got != tc.want {
			t.Errorf("%d.String(): got %q, want %q", tc.fmt, got, tc.want)
		}
	}
}

// Tests for MarkupLang String()
func TestMarkupLangString(t *testing.T) {
	testCases := []struct {
		markup MarkupLang
		want   string
	}{
		{MarkupNone, "None"},
		{MarkupHTML, "HTML"},
		{MarkupMarkdown, "Markdown"},
		{MarkupLaTeX, "LaTeX"},
		{MarkupRTF, "RTF"},
		{MarkupReST, "reST"},
		{MarkupAsciiDoc, "AsciiDoc"},
		{MarkupOrg, "Org"},
		{MarkupLang(99), "Unknown"},
	}

	for _, tc := range testCases {
		if got := tc.markup.String(); got != tc.want {
			t.Errorf("%d.String(): got %q, want %q", tc.markup, got, tc.want)
		}
	}
}

// Tests for NatLang String()
func TestNatLangString(t *testing.T) {
	testCases := []struct {
		lang NatLang
		want string
	}{
		{NatLangUnknown, "Unknown"},
		{NatLangEnglish, "English"},
		{NatLangSpanish, "Spanish"},
		{NatLangFrench, "French"},
		{NatLangPortuguese, "Portuguese"},
		{NatLangGerman, "German"},
		{NatLangItalian, "Italian"},
		{NatLangDutch, "Dutch"},
		{NatLangChinese, "Chinese"},
		{NatLangArabic, "Arabic"},
		{NatLangHindi, "Hindi"},
		{NatLangIndonesian, "Indonesian"},
		{NatLangBengali, "Bengali"},
		{NatLangRussian, "Russian"},
		{NatLangJapanese, "Japanese"},
		{NatLang(99), "Unknown"},
	}

	for _, tc := range testCases {
		if got := tc.lang.String(); got != tc.want {
			t.Errorf("%d.String(): got %q, want %q", tc.lang, got, tc.want)
		}
	}
}

// Tests for programming language detection
func TestDetectProgrammingLanguages(t *testing.T) {
	testCases := []struct {
		name string
		code string
		want CodeLang
	}{
		{
			name: "Go",
			code: `package main

import "fmt"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
	}
	defer cleanup()
}`,
			want: CodeLangGo,
		},
		{
			name: "Python",
			code: `import json
from typing import List

class MyClass:
    def __init__(self, name):
        self.name = name
    
    def process(self):
        if __name__ == "__main__":
            print("Hello")`,
			want: CodeLangPython,
		},
		{
			name: "JavaScript",
			code: `const express = require('express');
const app = express();

app.get('/', (req, res) => {
    console.log('Request received');
    res.send('Hello');
});

module.exports = app;`,
			want: CodeLangJavaScript,
		},
		{
			name: "Java",
			code: `import java.util.ArrayList;
import java.util.List;

public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello, World!");
    }
    
    @Override
    public String toString() {
        return "test";
    }
}`,
			want: CodeLangJava,
		},
		{
			name: "C",
			code: `#include <stdio.h>
#include <stdlib.h>

int main(int argc, char *argv[]) {
    printf("Hello, World!\n");
    int *ptr = malloc(sizeof(int) * 10);
    free(ptr);
    return 0;
}`,
			want: CodeLangC,
		},
		{
			name: "C++",
			code: `#include <iostream>
#include <vector>

using namespace std;

int main() {
    cout << "Hello" << endl;
    vector<int> v;
    v.push_back(1);
    return 0;
}`,
			want: CodeLangCPP,
		},
		{
			name: "C#",
			code: `using System;
using System.Collections.Generic;

namespace MyApp {
    public class Program {
        public static void Main(string[] args) {
            Console.WriteLine("Hello");
        }
        
        public string Name { get; set; }
    }
}`,
			want: CodeLangCSharp,
		},
		{
			name: "Ruby",
			code: `#!/usr/bin/env ruby
require 'json'
require 'net/http'

class HelloWorld
  attr_accessor :name, :value
  
  def initialize(name)
    @name = name
    @items = []
    @count = 0
  end
  
  def greet
    puts "Hello, #{@name}!"
    return true
  end
  
  def process(data)
    result = []
    data.each do |item|
      result << item * 2
    end
    result.map { |x| x + 1 }
  end
  
  def calculate(a, b)
    c = a + b - (a * b) / 2
    d = c * 3 + a - b
    return c + d
  end
  
  def check(x)
    if x > 0 && x < 100
      return true
    elsif x >= 100
      return false
    end
    nil
  end
end

obj = HelloWorld.new("World")
obj.greet
puts obj.calculate(10, 20)`,
			want: CodeLangRuby,
		},
		{
			name: "Rust",
			code: `use std::io;

fn main() {
    println!("Hello, World!");
    let mut x: i32 = 42;
    
    match x {
        0 => println!("zero"),
        _ => println!("other"),
    }
    
    impl MyTrait for MyStruct {
        fn method(&self) {}
    }
}`,
			want: CodeLangRust,
		},
		{
			name: "PHP",
			code: `<?php
namespace MyApp;

class HelloWorld {
    private $name;
    
    public function __construct() {
        $this->name = "World";
    }
    
    public function greet() {
        echo "Hello " . $this->name;
    }
}`,
			want: CodeLangPHP,
		},
		{
			name: "Swift",
			code: `import Foundation
import UIKit

class ViewController: UIViewController {
    @IBOutlet weak var label: UILabel!
    @IBAction func buttonTapped(_ sender: UIButton) {
        handleTap()
    }
    
    var count: Int = 0
    let maxItems: Int = 100
    
    override func viewDidLoad() {
        super.viewDidLoad()
        guard let name = getName() else { return }
        if let value = process() {
            print(value)
        }
        count += 1
    }
    
    func calculate(a: Int, b: Int) -> Int {
        return a + b - (a * b) / 2
    }
    
    func fetchData() {
        let url = URL(string: "https://api.example.com")!
        let task = URLSession.shared.dataTask(with: url) { data, response, error in
            guard let data = data else { return }
            print(data)
        }
        task.resume()
    }
}`,
			want: CodeLangSwift,
		},
		{
			name: "Kotlin",
			code: `import kotlin.collections.List

fun main(args: Array<String>) {
    println("Hello, World!")
    val name: String = "test"
    
    data class User(val name: String, val age: Int)
    
    user?.let { println(it.name) }
    
    suspend fun fetchData(): Result<String> {
        return Result.success("data")
    }
}`,
			want: CodeLangKotlin,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect([]byte(tc.code))
			if profile.Language != tc.want {
				t.Errorf("got %v, want %v", profile.Language, tc.want)
			}
		})
	}
}

// Tests for data format detection
func TestDetectDataFormats(t *testing.T) {
	testCases := []struct {
		name string
		data string
		want DataFormat
	}{
		{
			name: "JSON object",
			data: `{"name": "test", "value": 123, "items": ["a", "b"]}`,
			want: DataFormatJSON,
		},
		{
			name: "JSON array",
			data: `[{"id": 1}, {"id": 2}, {"id": 3}]`,
			want: DataFormatJSON,
		},
		{
			name: "XML with declaration",
			data: `<?xml version="1.0"?>
<root>
    <item>test</item>
</root>`,
			want: DataFormatXML,
		},
		{
			name: "XML without declaration",
			data: `<root>
    <item>test</item>
    <item>test2</item>
</root>`,
			want: DataFormatXML,
		},
		{
			name: "YAML with header",
			data: `---
name: test
items:
  - one
  - two
nested:
  key: value`,
			want: DataFormatYAML,
		},
		{
			name: "YAML simple",
			data: `name: test
value: 123
list:
  - item1
  - item2`,
			want: DataFormatYAML,
		},
		{
			name: "CSV",
			data: `name,age,city
John,30,NYC
Jane,25,LA
Bob,35,Chicago`,
			want: DataFormatCSV,
		},
		{
			name: "TOML",
			data: `[database]
server = "localhost"
port = 5432

[server]
host = "0.0.0.0"`,
			want: DataFormatTOML,
		},
		{
			name: "INI",
			data: `[section]
key=value
name=test

[another]
foo=bar`,
			want: DataFormatINI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect([]byte(tc.data))
			if profile.DataFmt != tc.want {
				t.Errorf("got %v, want %v", profile.DataFmt, tc.want)
			}
		})
	}
}

// Tests for markup detection
func TestDetectMarkup(t *testing.T) {
	testCases := []struct {
		name string
		data string
		want MarkupLang
	}{
		{
			name: "HTML with doctype",
			data: `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body><h1>Hello</h1></body>
</html>`,
			want: MarkupHTML,
		},
		{
			name: "HTML without doctype",
			data: `<html>
<head><title>Test</title></head>
<body>
<div class="container">
<p>Hello World</p>
</div>
</body>
</html>`,
			want: MarkupHTML,
		},
		{
			name: "Markdown",
			data: `# Hello World

This is a **bold** statement.

## Section

- Item 1
- Item 2

` + "```" + `python
print("hello")
` + "```" + `

[Link](https://example.com)`,
			want: MarkupMarkdown,
		},
		{
			name: "LaTeX",
			data: `\documentclass{article}
\usepackage{amsmath}

\begin{document}

\section{Introduction}

This is a test document.

\end{document}`,
			want: MarkupLaTeX,
		},
		{
			name: "RTF",
			data: `{\rtf1\ansi\deff0
{\fonttbl{\f0 Times New Roman;}}
\f0\fs24 Hello World!
}`,
			want: MarkupRTF,
		},
		{
			name: "Org-mode",
			data: `#+TITLE: My Document
#+AUTHOR: Test

* Introduction

This is the introduction.

#+BEGIN_SRC python
print("hello")
#+END_SRC`,
			want: MarkupOrg,
		},
		{
			name: "AsciiDoc",
			data: `= Document Title
:toc:

== Section One

This is content.

[source,python]
----
print("hello")
----`,
			want: MarkupAsciiDoc,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect([]byte(tc.data))
			if profile.Markup != tc.want {
				t.Errorf("got %v, want %v", profile.Markup, tc.want)
			}
		})
	}
}

// Tests for natural language detection
func TestDetectNatLang(t *testing.T) {
	testCases := []struct {
		name string
		text string
		want NatLang
	}{
		{
			name: "English",
			text: "The quick brown fox jumps over the lazy dog. This is a test of the English language detection system. It should work correctly for longer texts.",
			want: NatLangEnglish,
		},
		{
			name: "Spanish",
			text: "El rápido zorro marrón salta sobre el perro perezoso. Esta es una prueba del sistema de detección del idioma español. Debería funcionar correctamente para textos más largos.",
			want: NatLangSpanish,
		},
		{
			name: "French",
			text: "Le renard brun rapide saute par-dessus le chien paresseux. C'est un test du système de détection de la langue française. Il devrait fonctionner correctement pour les textes plus longs.",
			want: NatLangFrench,
		},
		{
			name: "German",
			text: "Der schnelle braune Fuchs springt über den faulen Hund. Dies ist ein Test des deutschen Spracherkennungssystems. Es sollte für längere Texte korrekt funktionieren.",
			want: NatLangGerman,
		},
		{
			name: "Portuguese",
			text: "A rápida raposa marrom salta sobre o cão preguiçoso. Este é um teste do sistema de detecção de idioma português. Não deve falhar para textos mais longos.",
			want: NatLangPortuguese,
		},
		{
			name: "Italian",
			text: "La veloce volpe marrone salta sopra il cane pigro. Questo è un test del sistema di rilevamento della lingua italiana. Non dovrebbe fallire per testi più lunghi.",
			want: NatLangItalian,
		},
		{
			name: "Dutch",
			text: "De snelle bruine vos springt over de luie hond. Dit is een test van het Nederlandse taaldetectiesysteem. Het zou correct moeten werken voor langere teksten.",
			want: NatLangDutch,
		},
		{
			name: "Chinese",
			text: "快速的棕色狐狸跳过了懒狗。这是中文语言检测系统的测试。对于较长的文本，它应该可以正常工作。",
			want: NatLangChinese,
		},
		{
			name: "Japanese",
			text: "素早い茶色のキツネが怠惰な犬を飛び越えます。これは日本語検出システムのテストです。長いテキストでも正しく動作するはずです。",
			want: NatLangJapanese,
		},
		{
			name: "Arabic",
			text: "الثعلب البني السريع يقفز فوق الكلب الكسول. هذا اختبار لنظام اكتشاف اللغة العربية. يجب أن يعمل بشكل صحيح للنصوص الأطول.",
			want: NatLangArabic,
		},
		{
			name: "Russian",
			text: "Быстрая коричневая лиса прыгает через ленивую собаку. Это тест системы определения русского языка. Он должен работать правильно для более длинных текстов.",
			want: NatLangRussian,
		},
		{
			name: "Indonesian",
			text: "Rubah coklat yang cepat melompati anjing yang malas. Ini adalah tes dari sistem deteksi bahasa Indonesia. Ini seharusnya bekerja dengan benar untuk teks yang lebih panjang.",
			want: NatLangIndonesian,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := Detect([]byte(tc.text))
			if profile.NatLang != tc.want {
				t.Errorf("got %v, want %v", profile.NatLang, tc.want)
			}
		})
	}
}

// Test decodeRune helper
func TestDecodeRune(t *testing.T) {
	testCases := []struct {
		name    string
		input   []byte
		wantR   rune
		wantLen int
	}{
		{"ASCII", []byte("A"), 'A', 1},
		{"2-byte UTF-8", []byte("é"), 'é', 2},
		{"3-byte UTF-8", []byte("中"), '中', 3},
		{"4-byte UTF-8", []byte("𝄞"), '𝄞', 4},
		{"Empty", []byte{}, 0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, n := decodeRune(tc.input)
			if r != tc.wantR || n != tc.wantLen {
				t.Errorf("decodeRune(%q): got (%U, %d), want (%U, %d)",
					tc.input, r, n, tc.wantR, tc.wantLen)
			}
		})
	}
}

// Test empty/edge cases for new detection functions
func TestDetectEdgeCases(t *testing.T) {
	// Empty data
	profile := Detect([]byte{})
	if profile.DataFmt != DataFormatNone {
		t.Errorf("empty: DataFmt got %v, want None", profile.DataFmt)
	}
	if profile.Markup != MarkupNone {
		t.Errorf("empty: Markup got %v, want None", profile.Markup)
	}
	if profile.NatLang != NatLangUnknown {
		t.Errorf("empty: NatLang got %v, want Unknown", profile.NatLang)
	}

	// Whitespace only
	profile = Detect([]byte("   \n\t  \n  "))
	if profile.DataFmt != DataFormatNone {
		t.Errorf("whitespace: DataFmt got %v, want None", profile.DataFmt)
	}

	// Short ambiguous text
	profile = Detect([]byte("hello"))
	if profile.NatLang != NatLangUnknown {
		t.Errorf("short: NatLang got %v, want Unknown", profile.NatLang)
	}
}
