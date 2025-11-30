// Command benchmark generates comprehensive compression benchmarks across
// multiple content types and sizes, producing HTML and JSON reports.
//
// Usage:
//
//	benchmark [-o output_dir] [-sizes 2,8,32,128,512,2048]
//
// Output files:
//   - report.json: Machine-readable benchmark data
//   - report.html: Interactive HTML report with charts
package main

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ha1tch/unz/pkg/bpe"
	"github.com/ha1tch/unz/pkg/compress"
	"github.com/ha1tch/unz/pkg/vocab"
)

// Content categories
type Category string

const (
	CatNaturalLang Category = "natural_language"
	CatProgLang    Category = "programming_language"
	CatStructured  Category = "structured_data"
	CatMarkup      Category = "markup"
)

// ContentType represents a specific content type within a category
type ContentType struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category Category `json:"category"`
	FileExt  string   `json:"file_ext"`
}

var contentTypes = []ContentType{
	// Natural languages
	{ID: "en", Name: "English", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "es", Name: "Spanish", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "fr", Name: "French", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "de", Name: "German", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "zh", Name: "Chinese", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "ru", Name: "Russian", Category: CatNaturalLang, FileExt: ".txt"},
	{ID: "ja", Name: "Japanese", Category: CatNaturalLang, FileExt: ".txt"},

	// Programming languages
	{ID: "go", Name: "Go", Category: CatProgLang, FileExt: ".go"},
	{ID: "python", Name: "Python", Category: CatProgLang, FileExt: ".py"},
	{ID: "javascript", Name: "JavaScript", Category: CatProgLang, FileExt: ".js"},
	{ID: "java", Name: "Java", Category: CatProgLang, FileExt: ".java"},
	{ID: "c", Name: "C", Category: CatProgLang, FileExt: ".c"},
	{ID: "rust", Name: "Rust", Category: CatProgLang, FileExt: ".rs"},
	{ID: "cpp", Name: "C++", Category: CatProgLang, FileExt: ".cpp"},

	// Structured data
	{ID: "json", Name: "JSON", Category: CatStructured, FileExt: ".json"},
	{ID: "xml", Name: "XML", Category: CatStructured, FileExt: ".xml"},
	{ID: "yaml", Name: "YAML", Category: CatStructured, FileExt: ".yaml"},
	{ID: "csv", Name: "CSV", Category: CatStructured, FileExt: ".csv"},

	// Markup
	{ID: "html", Name: "HTML", Category: CatMarkup, FileExt: ".html"},
	{ID: "markdown", Name: "Markdown", Category: CatMarkup, FileExt: ".md"},
	{ID: "latex", Name: "LaTeX", Category: CatMarkup, FileExt: ".tex"},
}

// BenchmarkResult holds results for a single benchmark run
type BenchmarkResult struct {
	ContentType string `json:"content_type"`
	ContentName string `json:"content_name"`
	Category    string `json:"category"`
	SizeKB      int    `json:"size_kb"`
	OriginalB   int    `json:"original_bytes"`

	// Compression results (compressed size in bytes)
	EnzSize     int    `json:"enz_size"`
	EnzMethod   string `json:"enz_method"`
	ZipSize     int    `json:"zip_size"`
	GzipSize    int    `json:"gzip_size"`
	DeflateOnly int    `json:"deflate_only"` // Raw DEFLATE (no container)
	BpelateOnly int    `json:"bpelate_only"` // Raw BPELATE (no container)
	UnzlateOnly int    `json:"unzlate_only"` // Raw UNZLATE (no container)

	// Derived metrics
	EnzRatio  float64 `json:"enz_ratio"` // Compression ratio (0-1, lower is better)
	ZipRatio  float64 `json:"zip_ratio"`
	GzipRatio float64 `json:"gzip_ratio"`
	EnzVsZip  float64 `json:"enz_vs_zip"` // % improvement over zip (positive = enz wins)
	EnzVsGzip float64 `json:"enz_vs_gzip"`
	Winner    string  `json:"winner"` // "enz", "zip", or "gzip"
}

// Report holds the complete benchmark report
type Report struct {
	Generated time.Time         `json:"generated"`
	GoVersion string            `json:"go_version"`
	Platform  string            `json:"platform"`
	SizesKB   []int             `json:"sizes_kb"`
	Results   []BenchmarkResult `json:"results"`
	Summary   ReportSummary     `json:"summary"`
}

// ReportSummary provides aggregate statistics
type ReportSummary struct {
	TotalTests  int                   `json:"total_tests"`
	EnzWins     int                   `json:"enz_wins"`
	ZipWins     int                   `json:"zip_wins"`
	GzipWins    int                   `json:"gzip_wins"`
	AvgEnzVsZip float64               `json:"avg_enz_vs_zip"`
	ByCategory  map[string]CatSummary `json:"by_category"`
	BySizeKB    map[int]SizeSummary   `json:"by_size_kb"`
}

type CatSummary struct {
	Tests       int     `json:"tests"`
	EnzWins     int     `json:"enz_wins"`
	AvgEnzVsZip float64 `json:"avg_enz_vs_zip"`
}

type SizeSummary struct {
	Tests       int     `json:"tests"`
	EnzWins     int     `json:"enz_wins"`
	AvgEnzVsZip float64 `json:"avg_enz_vs_zip"`
}

func main() {
	outputDir := flag.String("o", ".", "Output directory for reports")
	sizesFlag := flag.String("sizes", "2,4,8,16,32,64,128,256,512,1024,2048", "Comma-separated sizes in KB")
	flag.Parse()

	// Parse sizes
	var sizes []int
	for _, s := range strings.Split(*sizesFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid size: %s\n", s)
			os.Exit(1)
		}
		sizes = append(sizes, n)
	}
	sort.Ints(sizes)

	fmt.Printf("Compression Benchmark Report Generator\n")
	fmt.Printf("======================================\n")
	fmt.Printf("Sizes: %v KB\n", sizes)
	fmt.Printf("Content types: %d\n", len(contentTypes))
	fmt.Printf("Total benchmarks: %d\n\n", len(sizes)*len(contentTypes))

	// Create compressor
	v := vocab.ForLanguage(vocab.LangText)
	comp := compress.New(v)

	// Run benchmarks
	var results []BenchmarkResult
	total := len(sizes) * len(contentTypes)
	done := 0

	for _, ct := range contentTypes {
		for _, sizeKB := range sizes {
			done++
			fmt.Printf("\r[%d/%d] Testing %s at %d KB...", done, total, ct.Name, sizeKB)

			result := runBenchmark(comp, ct, sizeKB)
			results = append(results, result)
		}
	}
	fmt.Printf("\r[%d/%d] Complete!                              \n\n", total, total)

	// Build report
	report := Report{
		Generated: time.Now().UTC(),
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		SizesKB:   sizes,
		Results:   results,
		Summary:   calculateSummary(results),
	}

	// Write JSON report
	jsonPath := filepath.Join(*outputDir, "report.json")
	jsonData, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", jsonPath)

	// Write HTML report
	htmlPath := filepath.Join(*outputDir, "report.html")
	if err := writeHTMLReport(htmlPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing HTML: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", htmlPath)

	// Print summary
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total tests: %d\n", report.Summary.TotalTests)
	fmt.Printf("enz wins:    %d (%.1f%%)\n", report.Summary.EnzWins, 100*float64(report.Summary.EnzWins)/float64(report.Summary.TotalTests))
	fmt.Printf("zip wins:    %d (%.1f%%)\n", report.Summary.ZipWins, 100*float64(report.Summary.ZipWins)/float64(report.Summary.TotalTests))
	fmt.Printf("Avg enz vs zip: %.1f%%\n", report.Summary.AvgEnzVsZip)
}

func runBenchmark(comp *compress.Compressor, ct ContentType, sizeKB int) BenchmarkResult {
	targetSize := sizeKB * 1024
	data := generateContent(ct, targetSize)

	result := BenchmarkResult{
		ContentType: ct.ID,
		ContentName: ct.Name,
		Category:    string(ct.Category),
		SizeKB:      sizeKB,
		OriginalB:   len(data),
	}

	// Test enz (our compressor)
	enzZip, err := comp.CompressFile(data, "test"+ct.FileExt, time.Now())
	if err == nil {
		result.EnzSize = len(enzZip)
		result.EnzMethod = detectMethod(enzZip)
	}

	// Test zip -9 (standard ZIP with DEFLATE)
	result.ZipSize = testZip(data, ct.FileExt)

	// Test gzip -9
	result.GzipSize = testGzip(data)

	// Raw algorithm tests (no container overhead)
	result.DeflateOnly = len(deflateBytes(data))

	// BPELATE raw
	v := vocab.ForLanguage(vocab.LangText)
	encoder := bpe.NewEncoder(v)
	tokens := encoder.Encode(data)
	tokenBytes := encodeVarints(tokens)
	result.BpelateOnly = len(deflateBytes(tokenBytes))

	// Calculate metrics
	result.EnzRatio = float64(result.EnzSize) / float64(result.OriginalB)
	result.ZipRatio = float64(result.ZipSize) / float64(result.OriginalB)
	result.GzipRatio = float64(result.GzipSize) / float64(result.OriginalB)

	if result.ZipSize > 0 {
		result.EnzVsZip = 100 * (1 - float64(result.EnzSize)/float64(result.ZipSize))
	}
	if result.GzipSize > 0 {
		result.EnzVsGzip = 100 * (1 - float64(result.EnzSize)/float64(result.GzipSize))
	}

	// Determine winner (comparing same-format: enz vs zip)
	if result.EnzSize <= result.ZipSize {
		result.Winner = "enz"
	} else {
		result.Winner = "zip"
	}

	return result
}

func detectMethod(zipData []byte) string {
	if len(zipData) < 30 {
		return "unknown"
	}
	// Method is at offset 8 in local file header
	method := uint16(zipData[8]) | uint16(zipData[9])<<8
	switch method {
	case 0:
		return "Stored"
	case 8:
		return "Deflate"
	case 85:
		return "Unzlate"
	case 86:
		return "Bpelate"
	default:
		return fmt.Sprintf("Method%d", method)
	}
}

func testZip(data []byte, ext string) int {
	tmpDir, _ := os.MkdirTemp("", "bench")
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "test"+ext)
	outPath := filepath.Join(tmpDir, "test.zip")

	os.WriteFile(inPath, data, 0644)
	cmd := exec.Command("zip", "-9", "-q", outPath, inPath)
	cmd.Run()

	info, err := os.Stat(outPath)
	if err != nil {
		return 0
	}
	return int(info.Size())
}

func testGzip(data []byte) int {
	tmpDir, _ := os.MkdirTemp("", "bench")
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "test.dat")
	outPath := filepath.Join(tmpDir, "test.gz")

	os.WriteFile(inPath, data, 0644)
	cmd := exec.Command("gzip", "-9", "-c", inPath)
	out, _ := cmd.Output()
	os.WriteFile(outPath, out, 0644)

	return len(out)
}

func deflateBytes(data []byte) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func encodeVarints(tokens []int) []byte {
	var buf bytes.Buffer
	for _, t := range tokens {
		if t < 128 {
			buf.WriteByte(byte(t))
		} else if t < 16384 {
			buf.WriteByte(byte(t&0x7F) | 0x80)
			buf.WriteByte(byte(t >> 7))
		} else {
			buf.WriteByte(byte(t&0x7F) | 0x80)
			buf.WriteByte(byte((t>>7)&0x7F) | 0x80)
			buf.WriteByte(byte(t >> 14))
		}
	}
	return buf.Bytes()
}

func calculateSummary(results []BenchmarkResult) ReportSummary {
	summary := ReportSummary{
		TotalTests: len(results),
		ByCategory: make(map[string]CatSummary),
		BySizeKB:   make(map[int]SizeSummary),
	}

	var totalEnzVsZip float64

	for _, r := range results {
		totalEnzVsZip += r.EnzVsZip

		if r.Winner == "enz" {
			summary.EnzWins++
		} else if r.Winner == "zip" {
			summary.ZipWins++
		} else {
			summary.GzipWins++
		}

		// By category
		cat := summary.ByCategory[r.Category]
		cat.Tests++
		cat.AvgEnzVsZip += r.EnzVsZip
		if r.Winner == "enz" {
			cat.EnzWins++
		}
		summary.ByCategory[r.Category] = cat

		// By size
		size := summary.BySizeKB[r.SizeKB]
		size.Tests++
		size.AvgEnzVsZip += r.EnzVsZip
		if r.Winner == "enz" {
			size.EnzWins++
		}
		summary.BySizeKB[r.SizeKB] = size
	}

	// Calculate averages
	if len(results) > 0 {
		summary.AvgEnzVsZip = totalEnzVsZip / float64(len(results))
	}

	for k, v := range summary.ByCategory {
		if v.Tests > 0 {
			v.AvgEnzVsZip /= float64(v.Tests)
			summary.ByCategory[k] = v
		}
	}

	for k, v := range summary.BySizeKB {
		if v.Tests > 0 {
			v.AvgEnzVsZip /= float64(v.Tests)
			summary.BySizeKB[k] = v
		}
	}

	return summary
}

// === Content Generators ===

func generateContent(ct ContentType, targetSize int) []byte {
	switch ct.Category {
	case CatNaturalLang:
		return generateNaturalLang(ct.ID, targetSize)
	case CatProgLang:
		return generateCode(ct.ID, targetSize)
	case CatStructured:
		return generateStructured(ct.ID, targetSize)
	case CatMarkup:
		return generateMarkup(ct.ID, targetSize)
	default:
		return bytes.Repeat([]byte("x"), targetSize)
	}
}

func generateNaturalLang(lang string, targetSize int) []byte {
	var corpus []string

	switch lang {
	case "en":
		corpus = []string{
			"The quick brown fox jumps over the lazy dog.",
			"In a world where technology advances rapidly, we must adapt.",
			"The economy continues to show signs of recovery.",
			"Scientists have discovered a new species in the deep ocean.",
			"The government announced new policies to address climate change.",
			"Education remains a cornerstone of societal development.",
			"Healthcare systems around the world face unprecedented challenges.",
			"The arts and culture sector is experiencing a renaissance.",
			"Innovation drives progress in every industry and field.",
			"Community engagement is essential for local development.",
		}
	case "es":
		corpus = []string{
			"El rápido zorro marrón salta sobre el perro perezoso.",
			"En un mundo donde la tecnología avanza rápidamente, debemos adaptarnos.",
			"La economía continúa mostrando signos de recuperación.",
			"Los científicos han descubierto una nueva especie en el océano profundo.",
			"El gobierno anunció nuevas políticas para abordar el cambio climático.",
			"La educación sigue siendo la piedra angular del desarrollo social.",
			"Los sistemas de salud en todo el mundo enfrentan desafíos sin precedentes.",
			"El sector de las artes y la cultura está experimentando un renacimiento.",
			"La innovación impulsa el progreso en todas las industrias y campos.",
			"La participación comunitaria es esencial para el desarrollo local.",
		}
	case "fr":
		corpus = []string{
			"Le rapide renard brun saute par-dessus le chien paresseux.",
			"Dans un monde où la technologie progresse rapidement, nous devons nous adapter.",
			"L'économie continue de montrer des signes de reprise.",
			"Les scientifiques ont découvert une nouvelle espèce dans l'océan profond.",
			"Le gouvernement a annoncé de nouvelles politiques pour lutter contre le changement climatique.",
			"L'éducation reste la pierre angulaire du développement sociétal.",
			"Les systèmes de santé du monde entier font face à des défis sans précédent.",
			"Le secteur des arts et de la culture connaît une renaissance.",
			"L'innovation stimule le progrès dans tous les secteurs et domaines.",
			"L'engagement communautaire est essentiel au développement local.",
		}
	case "de":
		corpus = []string{
			"Der schnelle braune Fuchs springt über den faulen Hund.",
			"In einer Welt, in der die Technologie schnell voranschreitet, müssen wir uns anpassen.",
			"Die Wirtschaft zeigt weiterhin Anzeichen einer Erholung.",
			"Wissenschaftler haben eine neue Spezies in der Tiefsee entdeckt.",
			"Die Regierung kündigte neue Maßnahmen zur Bekämpfung des Klimawandels an.",
			"Bildung bleibt ein Eckpfeiler der gesellschaftlichen Entwicklung.",
			"Gesundheitssysteme auf der ganzen Welt stehen vor beispiellosen Herausforderungen.",
			"Der Kunst- und Kultursektor erlebt eine Renaissance.",
			"Innovation treibt den Fortschritt in allen Branchen und Bereichen voran.",
			"Gemeinschaftliches Engagement ist für die lokale Entwicklung unerlässlich.",
		}
	case "zh":
		corpus = []string{
			"快速的棕色狐狸跳过懒狗。",
			"在技术快速发展的世界中，我们必须适应。",
			"经济继续显示复苏迹象。",
			"科学家在深海发现了一个新物种。",
			"政府宣布了应对气候变化的新政策。",
			"教育仍然是社会发展的基石。",
			"世界各地的医疗系统面临前所未有的挑战。",
			"艺术和文化领域正在经历复兴。",
			"创新推动着每个行业和领域的进步。",
			"社区参与对当地发展至关重要。",
		}
	case "ru":
		corpus = []string{
			"Быстрая коричневая лиса перепрыгивает через ленивую собаку.",
			"В мире, где технологии быстро развиваются, мы должны адаптироваться.",
			"Экономика продолжает показывать признаки восстановления.",
			"Ученые обнаружили новый вид в глубинах океана.",
			"Правительство объявило о новой политике по борьбе с изменением климата.",
			"Образование остается краеугольным камнем общественного развития.",
			"Системы здравоохранения по всему миру сталкиваются с беспрецедентными проблемами.",
			"Сектор искусства и культуры переживает возрождение.",
			"Инновации движут прогрессом во всех отраслях и сферах.",
			"Участие сообщества имеет важное значение для местного развития.",
		}
	case "ja":
		corpus = []string{
			"素早い茶色の狐が怠惰な犬を飛び越える。",
			"技術が急速に進歩する世界では、私たちは適応しなければなりません。",
			"経済は回復の兆しを見せ続けています。",
			"科学者たちは深海で新種を発見しました。",
			"政府は気候変動に対処するための新しい政策を発表しました。",
			"教育は社会発展の礎石であり続けています。",
			"世界中の医療システムは前例のない課題に直面しています。",
			"芸術と文化のセクターはルネッサンスを経験しています。",
			"イノベーションはあらゆる産業と分野で進歩を推進します。",
			"コミュニティの関与は地域開発に不可欠です。",
		}
	default:
		corpus = []string{"Lorem ipsum dolor sit amet, consectetur adipiscing elit."}
	}

	var sb strings.Builder
	rng := rand.New(rand.NewSource(42))
	for sb.Len() < targetSize {
		sb.WriteString(corpus[rng.Intn(len(corpus))])
		sb.WriteString(" ")
	}
	return []byte(sb.String()[:targetSize])
}

func generateCode(lang string, targetSize int) []byte {
	var sb strings.Builder
	rng := rand.New(rand.NewSource(42))

	switch lang {
	case "go":
		sb.WriteString("package main\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n\t\"errors\"\n)\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(goFunction(rng))
		}
	case "python":
		sb.WriteString("#!/usr/bin/env python3\nimport json\nimport asyncio\nfrom typing import List, Optional, Dict\nfrom dataclasses import dataclass\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(pythonFunction(rng))
		}
	case "javascript":
		sb.WriteString("'use strict';\nconst express = require('express');\nconst { useState, useEffect } = require('react');\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(jsFunction(rng))
		}
	case "java":
		sb.WriteString("package com.example.app;\n\nimport java.util.*;\nimport java.io.*;\n\npublic class Application {\n")
		for sb.Len() < targetSize-2 {
			sb.WriteString(javaMethod(rng))
		}
		sb.WriteString("}\n")
	case "c":
		sb.WriteString("#include <stdio.h>\n#include <stdlib.h>\n#include <string.h>\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(cFunction(rng))
		}
	case "rust":
		sb.WriteString("use std::collections::HashMap;\nuse std::io::{self, Read, Write};\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(rustFunction(rng))
		}
	case "cpp":
		sb.WriteString("#include <iostream>\n#include <vector>\n#include <string>\n#include <memory>\n\n")
		for sb.Len() < targetSize {
			sb.WriteString(cppFunction(rng))
		}
	}

	result := sb.String()
	if len(result) > targetSize {
		result = result[:targetSize]
	}
	return []byte(result)
}

func goFunction(rng *rand.Rand) string {
	names := []string{"Process", "Handle", "Validate", "Transform", "Execute", "Calculate", "Parse", "Format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`func %s%d(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}
	result := make([]byte, len(data))
	for i, b := range data {
		result[i] = b ^ byte(i%%256)
	}
	if err := validate(result); err != nil {
		return nil, fmt.Errorf("%s%d: %%w", err)
	}
	return result, nil
}

`, name, id, name, id)
}

func pythonFunction(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`async def %s_%d(data: List[Dict]) -> Optional[Dict]:
    """Process the input data and return results."""
    if not data:
        raise ValueError("Empty input")
    
    results = []
    for item in data:
        if "id" not in item:
            continue
        processed = {
            "id": item["id"],
            "value": item.get("value", 0) * 2,
            "status": "processed"
        }
        results.append(processed)
    
    return {"count": len(results), "items": results}


`, name, id)
}

func jsFunction(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`async function %s%d(data) {
    if (!data || !Array.isArray(data)) {
        throw new Error('Invalid input');
    }
    
    const results = await Promise.all(
        data.map(async (item) => {
            const processed = await transformItem(item);
            return {
                ...processed,
                timestamp: Date.now(),
                status: 'complete'
            };
        })
    );
    
    return { success: true, count: results.length, data: results };
}

`, name, id)
}

func javaMethod(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`    public List<Map<String, Object>> %s%d(List<Map<String, Object>> data) {
        if (data == null || data.isEmpty()) {
            throw new IllegalArgumentException("Empty input");
        }
        
        List<Map<String, Object>> results = new ArrayList<>();
        for (Map<String, Object> item : data) {
            Map<String, Object> processed = new HashMap<>();
            processed.put("id", item.get("id"));
            processed.put("value", ((Integer) item.getOrDefault("value", 0)) * 2);
            processed.put("status", "processed");
            results.add(processed);
        }
        
        return results;
    }

`, name, id)
}

func cFunction(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`int %s_%d(const char *input, size_t len, char *output) {
    if (input == NULL || output == NULL || len == 0) {
        return -1;
    }
    
    for (size_t i = 0; i < len; i++) {
        output[i] = input[i] ^ (i %% 256);
    }
    
    printf("%s_%d: processed %%zu bytes\n", len);
    return 0;
}

`, name, id, name, id)
}

func rustFunction(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`fn %s_%d(data: &[u8]) -> Result<Vec<u8>, Box<dyn std::error::Error>> {
    if data.is_empty() {
        return Err("Empty input".into());
    }
    
    let result: Vec<u8> = data
        .iter()
        .enumerate()
        .map(|(i, &b)| b ^ (i as u8))
        .collect();
    
    println!("%s_%d: processed {} bytes", data.len());
    Ok(result)
}

`, name, id, name, id)
}

func cppFunction(rng *rand.Rand) string {
	names := []string{"process", "handle", "validate", "transform", "execute", "calculate", "parse", "format"}
	name := names[rng.Intn(len(names))]
	id := rng.Intn(1000)
	return fmt.Sprintf(`std::vector<uint8_t> %s_%d(const std::vector<uint8_t>& data) {
    if (data.empty()) {
        throw std::invalid_argument("Empty input");
    }
    
    std::vector<uint8_t> result;
    result.reserve(data.size());
    
    for (size_t i = 0; i < data.size(); ++i) {
        result.push_back(data[i] ^ static_cast<uint8_t>(i));
    }
    
    std::cout << "%s_%d: processed " << data.size() << " bytes" << std::endl;
    return result;
}

`, name, id, name, id)
}

func generateStructured(format string, targetSize int) []byte {
	var sb strings.Builder
	rng := rand.New(rand.NewSource(42))

	switch format {
	case "json":
		sb.WriteString(`{"data":[`)
		first := true
		for sb.Len() < targetSize-50 {
			if !first {
				sb.WriteString(",")
			}
			first = false
			id := rng.Intn(100000)
			fmt.Fprintf(&sb, `{"id":%d,"name":"User %d","email":"user%d@example.com","active":%v,"score":%d,"tags":["tag%d","tag%d"]}`,
				id, id, id, rng.Intn(2) == 1, rng.Intn(100), rng.Intn(10), rng.Intn(10))
		}
		sb.WriteString(`],"meta":{"count":1,"page":1}}`)

	case "xml":
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<root>
  <data>
`)
		for sb.Len() < targetSize-50 {
			id := rng.Intn(100000)
			fmt.Fprintf(&sb, `    <item id="%d">
      <name>User %d</name>
      <email>user%d@example.com</email>
      <active>%v</active>
      <score>%d</score>
    </item>
`, id, id, id, rng.Intn(2) == 1, rng.Intn(100))
		}
		sb.WriteString(`  </data>
</root>`)

	case "yaml":
		sb.WriteString("---\ndata:\n")
		for sb.Len() < targetSize-20 {
			id := rng.Intn(100000)
			fmt.Fprintf(&sb, `  - id: %d
    name: "User %d"
    email: "user%d@example.com"
    active: %v
    score: %d
    tags:
      - tag%d
      - tag%d
`, id, id, id, rng.Intn(2) == 1, rng.Intn(100), rng.Intn(10), rng.Intn(10))
		}

	case "csv":
		sb.WriteString("id,name,email,active,score,created_at\n")
		for sb.Len() < targetSize {
			id := rng.Intn(100000)
			fmt.Fprintf(&sb, "%d,User %d,user%d@example.com,%v,%d,2024-%02d-%02d\n",
				id, id, id, rng.Intn(2) == 1, rng.Intn(100), rng.Intn(12)+1, rng.Intn(28)+1)
		}
	}

	result := sb.String()
	if len(result) > targetSize {
		result = result[:targetSize]
	}
	return []byte(result)
}

func generateMarkup(format string, targetSize int) []byte {
	var sb strings.Builder
	rng := rand.New(rand.NewSource(42))

	switch format {
	case "html":
		sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sample Document</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: #333; }
        p { line-height: 1.6; }
    </style>
</head>
<body>
    <div class="container">
`)
		for sb.Len() < targetSize-100 {
			id := rng.Intn(1000)
			fmt.Fprintf(&sb, `        <section id="section-%d">
            <h2>Section %d: Topic Overview</h2>
            <p>This is paragraph content for section %d. It contains information about various topics and demonstrates typical HTML structure.</p>
            <ul>
                <li>Item %d-1: First list item</li>
                <li>Item %d-2: Second list item</li>
                <li>Item %d-3: Third list item</li>
            </ul>
        </section>
`, id, id, id, id, id, id)
		}
		sb.WriteString(`    </div>
</body>
</html>`)

	case "markdown":
		sb.WriteString("# Main Document Title\n\n")
		for sb.Len() < targetSize {
			id := rng.Intn(1000)
			fmt.Fprintf(&sb, `## Section %d

This is a paragraph with some **bold text** and *italic text*. Here is a [link](https://example.com/%d) to more information.

### Subsection %d.1

- First bullet point with details
- Second bullet point with more content
- Third bullet point for completion

`+"```go\n"+`func example%d() {
    fmt.Println("Hello, World!")
}
`+"```\n\n", id, id, id, id)
		}

	case "latex":
		sb.WriteString(`\documentclass{article}
\usepackage[utf8]{inputenc}
\usepackage{amsmath}
\usepackage{graphicx}

\title{Sample Document}
\author{Author Name}
\date{\today}

\begin{document}
\maketitle

\begin{abstract}
This is the abstract of the document providing a brief overview of the content.
\end{abstract}

`)
		for sb.Len() < targetSize-50 {
			id := rng.Intn(1000)
			fmt.Fprintf(&sb, `\section{Section %d}

This section discusses topic %d in detail. The mathematical relationship can be expressed as:

\begin{equation}
    f_%d(x) = \sum_{i=0}^{n} a_i x^i + \epsilon
\end{equation}

\subsection{Subsection %d.1}

Additional content with inline math $E = mc^2$ and references to Figure~\ref{fig:%d}.

`, id, id, id, id, id)
		}
		sb.WriteString(`\end{document}`)
	}

	result := sb.String()
	if len(result) > targetSize {
		result = result[:targetSize]
	}
	return []byte(result)
}

// === HTML Report Template ===

func writeHTMLReport(path string, report Report) error {
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"pct":      func(f float64) string { return fmt.Sprintf("%.1f", f) },
		"kb":       func(b int) string { return fmt.Sprintf("%.1f", float64(b)/1024) },
		"positive": func(f float64) bool { return f > 0 },
		"lower":    strings.ToLower,
	}).Parse(htmlTemplate))

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, report)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Compression Benchmark Report</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        :root {
            --bg: #0d1117;
            --surface: #161b22;
            --border: #30363d;
            --text: #c9d1d9;
            --text-muted: #8b949e;
            --accent: #58a6ff;
            --green: #3fb950;
            --red: #f85149;
            --yellow: #d29922;
        }
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg);
            color: var(--text);
            margin: 0;
            padding: 2rem;
            line-height: 1.6;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        h1, h2, h3 { color: var(--text); margin-top: 2rem; }
        h1 { border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
        .meta {
            color: var(--text-muted);
            font-size: 0.9rem;
            margin-bottom: 2rem;
        }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin: 2rem 0;
        }
        .summary-card {
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 1.5rem;
            text-align: center;
        }
        .summary-card .value {
            font-size: 2.5rem;
            font-weight: bold;
            color: var(--accent);
        }
        .summary-card .label {
            color: var(--text-muted);
            font-size: 0.9rem;
        }
        .summary-card.win .value { color: var(--green); }
        .summary-card.loss .value { color: var(--red); }
        .chart-container {
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 1.5rem;
            margin: 1.5rem 0;
        }
        .chart-row {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
        }
        @media (max-width: 900px) {
            .chart-row { grid-template-columns: 1fr; }
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 1rem 0;
            font-size: 0.9rem;
        }
        th, td {
            padding: 0.75rem;
            text-align: left;
            border-bottom: 1px solid var(--border);
        }
        th {
            background: var(--surface);
            color: var(--text-muted);
            font-weight: 600;
            position: sticky;
            top: 0;
        }
        tr:hover { background: var(--surface); }
        .num { text-align: right; font-family: monospace; }
        .win { color: var(--green); }
        .loss { color: var(--red); }
        .method {
            display: inline-block;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.8rem;
            font-weight: 500;
        }
        .method-bpelate { background: #1f6feb33; color: #58a6ff; }
        .method-deflate { background: #3fb95033; color: #3fb950; }
        .method-unzlate { background: #d2992233; color: #d29922; }
        .method-stored { background: #8b949e33; color: #8b949e; }
        .tabs {
            display: flex;
            gap: 0;
            border-bottom: 1px solid var(--border);
            margin-bottom: 1rem;
        }
        .tab {
            padding: 0.75rem 1.5rem;
            cursor: pointer;
            border: none;
            background: none;
            color: var(--text-muted);
            font-size: 0.95rem;
            border-bottom: 2px solid transparent;
            margin-bottom: -1px;
        }
        .tab:hover { color: var(--text); }
        .tab.active {
            color: var(--accent);
            border-bottom-color: var(--accent);
        }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
        .filter-row {
            display: flex;
            gap: 1rem;
            margin-bottom: 1rem;
            flex-wrap: wrap;
        }
        select {
            background: var(--surface);
            border: 1px solid var(--border);
            color: var(--text);
            padding: 0.5rem 1rem;
            border-radius: 4px;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Compression Benchmark Report</h1>
        <div class="meta">
            Generated: {{.Generated.Format "2006-01-02 15:04:05 UTC"}} |
            Platform: {{.Platform}} |
            Go: {{.GoVersion}} |
            Sizes tested: {{range $i, $s := .SizesKB}}{{if $i}}, {{end}}{{$s}}KB{{end}}
        </div>

        <div class="summary-grid">
            <div class="summary-card">
                <div class="value">{{.Summary.TotalTests}}</div>
                <div class="label">Total Tests</div>
            </div>
            <div class="summary-card win">
                <div class="value">{{.Summary.EnzWins}}</div>
                <div class="label">enz Wins</div>
            </div>
            <div class="summary-card loss">
                <div class="value">{{.Summary.ZipWins}}</div>
                <div class="label">zip Wins</div>
            </div>
            <div class="summary-card {{if positive .Summary.AvgEnzVsZip}}win{{else}}loss{{end}}">
                <div class="value">{{pct .Summary.AvgEnzVsZip}}%</div>
                <div class="label">Avg enz vs zip</div>
            </div>
        </div>

        <div class="tabs">
            <button class="tab active" onclick="showTab('overview')">Overview</button>
            <button class="tab" onclick="showTab('by-category')">By Category</button>
            <button class="tab" onclick="showTab('by-size')">By Size</button>
            <button class="tab" onclick="showTab('all-results')">All Results</button>
        </div>

        <div id="overview" class="tab-content active">
            <div class="chart-row">
                <div class="chart-container">
                    <h3>Win Rate by Category</h3>
                    <canvas id="categoryWinChart"></canvas>
                </div>
                <div class="chart-container">
                    <h3>Average Improvement vs zip by Category</h3>
                    <canvas id="categoryImprovementChart"></canvas>
                </div>
            </div>
            <div class="chart-row">
                <div class="chart-container">
                    <h3>Win Rate by File Size</h3>
                    <canvas id="sizeWinChart"></canvas>
                </div>
                <div class="chart-container">
                    <h3>Average Improvement vs zip by Size</h3>
                    <canvas id="sizeImprovementChart"></canvas>
                </div>
            </div>
        </div>

        <div id="by-category" class="tab-content">
            <h2>Results by Category</h2>
            {{range $cat, $sum := .Summary.ByCategory}}
            <h3>{{$cat}}</h3>
            <p>Tests: {{$sum.Tests}} | enz wins: {{$sum.EnzWins}} | Avg improvement: {{pct $sum.AvgEnzVsZip}}%</p>
            <table>
                <thead>
                    <tr>
                        <th>Content</th>
                        <th>Size</th>
                        <th class="num">Original</th>
                        <th class="num">enz</th>
                        <th>Method</th>
                        <th class="num">zip</th>
                        <th class="num">vs zip</th>
                        <th>Winner</th>
                    </tr>
                </thead>
                <tbody>
                {{range $.Results}}{{if eq .Category $cat}}
                    <tr>
                        <td>{{.ContentName}}</td>
                        <td>{{.SizeKB}} KB</td>
                        <td class="num">{{.OriginalB}}</td>
                        <td class="num">{{.EnzSize}}</td>
                        <td><span class="method method-{{.EnzMethod | lower}}">{{.EnzMethod}}</span></td>
                        <td class="num">{{.ZipSize}}</td>
                        <td class="num {{if positive .EnzVsZip}}win{{else}}loss{{end}}">{{pct .EnzVsZip}}%</td>
                        <td class="{{if eq .Winner "enz"}}win{{else}}loss{{end}}">{{.Winner}}</td>
                    </tr>
                {{end}}{{end}}
                </tbody>
            </table>
            {{end}}
        </div>

        <div id="by-size" class="tab-content">
            <h2>Results by File Size</h2>
            {{range $size, $sum := .Summary.BySizeKB}}
            <h3>{{$size}} KB</h3>
            <p>Tests: {{$sum.Tests}} | enz wins: {{$sum.EnzWins}} | Avg improvement: {{pct $sum.AvgEnzVsZip}}%</p>
            {{end}}
            <div class="chart-container">
                <canvas id="sizeDetailChart" height="400"></canvas>
            </div>
        </div>

        <div id="all-results" class="tab-content">
            <h2>All Results</h2>
            <div class="filter-row">
                <select id="filterCategory" onchange="filterTable()">
                    <option value="">All Categories</option>
                    <option value="natural_language">Natural Language</option>
                    <option value="programming_language">Programming Language</option>
                    <option value="structured_data">Structured Data</option>
                    <option value="markup">Markup</option>
                </select>
                <select id="filterSize" onchange="filterTable()">
                    <option value="">All Sizes</option>
                    {{range .SizesKB}}<option value="{{.}}">{{.}} KB</option>{{end}}
                </select>
            </div>
            <table id="resultsTable">
                <thead>
                    <tr>
                        <th>Category</th>
                        <th>Content</th>
                        <th>Size</th>
                        <th class="num">Original</th>
                        <th class="num">enz</th>
                        <th>Method</th>
                        <th class="num">zip</th>
                        <th class="num">gzip</th>
                        <th class="num">vs zip</th>
                        <th>Winner</th>
                    </tr>
                </thead>
                <tbody>
                {{range .Results}}
                    <tr data-category="{{.Category}}" data-size="{{.SizeKB}}">
                        <td>{{.Category}}</td>
                        <td>{{.ContentName}}</td>
                        <td>{{.SizeKB}} KB</td>
                        <td class="num">{{.OriginalB}}</td>
                        <td class="num">{{.EnzSize}}</td>
                        <td><span class="method method-{{.EnzMethod | lower}}">{{.EnzMethod}}</span></td>
                        <td class="num">{{.ZipSize}}</td>
                        <td class="num">{{.GzipSize}}</td>
                        <td class="num {{if positive .EnzVsZip}}win{{else}}loss{{end}}">{{pct .EnzVsZip}}%</td>
                        <td class="{{if eq .Winner "enz"}}win{{else}}loss{{end}}">{{.Winner}}</td>
                    </tr>
                {{end}}
                </tbody>
            </table>
        </div>
    </div>

    <script>
        // Tab switching
        function showTab(tabId) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
            document.querySelector(` + "`" + `.tab[onclick="showTab('${tabId}')"]` + "`" + `).classList.add('active');
            document.getElementById(tabId).classList.add('active');
        }

        // Table filtering
        function filterTable() {
            const cat = document.getElementById('filterCategory').value;
            const size = document.getElementById('filterSize').value;
            document.querySelectorAll('#resultsTable tbody tr').forEach(row => {
                const matchCat = !cat || row.dataset.category === cat;
                const matchSize = !size || row.dataset.size === size;
                row.style.display = matchCat && matchSize ? '' : 'none';
            });
        }

        // Chart data from Go template
        const categoryData = {
            {{range $cat, $sum := .Summary.ByCategory}}
            "{{$cat}}": { tests: {{$sum.Tests}}, wins: {{$sum.EnzWins}}, avgImprovement: {{$sum.AvgEnzVsZip}} },
            {{end}}
        };

        const sizeData = {
            {{range $size, $sum := .Summary.BySizeKB}}
            "{{$size}}": { tests: {{$sum.Tests}}, wins: {{$sum.EnzWins}}, avgImprovement: {{$sum.AvgEnzVsZip}} },
            {{end}}
        };

        const chartColors = {
            green: '#3fb950',
            red: '#f85149',
            blue: '#58a6ff',
            yellow: '#d29922',
            gray: '#8b949e'
        };

        // Category win rate chart
        new Chart(document.getElementById('categoryWinChart'), {
            type: 'bar',
            data: {
                labels: Object.keys(categoryData),
                datasets: [{
                    label: 'enz Win %',
                    data: Object.values(categoryData).map(d => (d.wins / d.tests * 100).toFixed(1)),
                    backgroundColor: chartColors.green
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: { y: { beginAtZero: true, max: 100, ticks: { color: '#8b949e' } }, x: { ticks: { color: '#8b949e' } } }
            }
        });

        // Category improvement chart
        new Chart(document.getElementById('categoryImprovementChart'), {
            type: 'bar',
            data: {
                labels: Object.keys(categoryData),
                datasets: [{
                    label: 'Avg % improvement',
                    data: Object.values(categoryData).map(d => d.avgImprovement.toFixed(1)),
                    backgroundColor: Object.values(categoryData).map(d => d.avgImprovement >= 0 ? chartColors.green : chartColors.red)
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: { y: { ticks: { color: '#8b949e' } }, x: { ticks: { color: '#8b949e' } } }
            }
        });

        // Size win rate chart
        const sizeLabels = Object.keys(sizeData).sort((a, b) => parseInt(a) - parseInt(b));
        new Chart(document.getElementById('sizeWinChart'), {
            type: 'line',
            data: {
                labels: sizeLabels.map(s => s + ' KB'),
                datasets: [{
                    label: 'enz Win %',
                    data: sizeLabels.map(s => (sizeData[s].wins / sizeData[s].tests * 100).toFixed(1)),
                    borderColor: chartColors.blue,
                    backgroundColor: chartColors.blue + '33',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: { y: { beginAtZero: true, max: 100, ticks: { color: '#8b949e' } }, x: { ticks: { color: '#8b949e' } } }
            }
        });

        // Size improvement chart
        new Chart(document.getElementById('sizeImprovementChart'), {
            type: 'line',
            data: {
                labels: sizeLabels.map(s => s + ' KB'),
                datasets: [{
                    label: 'Avg % improvement',
                    data: sizeLabels.map(s => sizeData[s].avgImprovement.toFixed(1)),
                    borderColor: chartColors.green,
                    backgroundColor: chartColors.green + '33',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: { y: { ticks: { color: '#8b949e' } }, x: { ticks: { color: '#8b949e' } } }
            }
        });

        // Detailed size chart
        const allResults = [
            {{range .Results}}
            { cat: "{{.Category}}", content: "{{.ContentName}}", size: {{.SizeKB}}, enzVsZip: {{.EnzVsZip}} },
            {{end}}
        ];

        new Chart(document.getElementById('sizeDetailChart'), {
            type: 'scatter',
            data: {
                datasets: [
                    {
                        label: 'Natural Language',
                        data: allResults.filter(r => r.cat === 'natural_language').map(r => ({x: r.size, y: r.enzVsZip})),
                        backgroundColor: chartColors.blue
                    },
                    {
                        label: 'Programming',
                        data: allResults.filter(r => r.cat === 'programming_language').map(r => ({x: r.size, y: r.enzVsZip})),
                        backgroundColor: chartColors.green
                    },
                    {
                        label: 'Structured Data',
                        data: allResults.filter(r => r.cat === 'structured_data').map(r => ({x: r.size, y: r.enzVsZip})),
                        backgroundColor: chartColors.yellow
                    },
                    {
                        label: 'Markup',
                        data: allResults.filter(r => r.cat === 'markup').map(r => ({x: r.size, y: r.enzVsZip})),
                        backgroundColor: chartColors.red
                    }
                ]
            },
            options: {
                responsive: true,
                scales: {
                    x: { type: 'logarithmic', title: { display: true, text: 'File Size (KB)', color: '#8b949e' }, ticks: { color: '#8b949e' } },
                    y: { title: { display: true, text: 'enz vs zip (%)', color: '#8b949e' }, ticks: { color: '#8b949e' } }
                },
                plugins: {
                    legend: { labels: { color: '#c9d1d9' } }
                }
            }
        });
    </script>
</body>
</html>`
