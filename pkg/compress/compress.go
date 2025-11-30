// Package compress provides ZIP-compatible compression with UNZLATE support.
//
// The package reads and writes standard PKZIP format files. It supports:
//   - Method 0: Stored (no compression)
//   - Method 8: DEFLATE (standard ZIP compression)
//   - Method 85: UNZLATE (BPE + ANS, proprietary extension)
//
// Standard ZIP tools can list archives and extract Stored/DEFLATE entries.
// UNZLATE entries will report "unsupported compression method 85".
//
// Extended features:
//   - UTF-8 filenames (flag bit 11)
//   - Unix timestamps (extra field 0x5455)
//   - Unix permissions (external attributes)
package compress

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ha1tch/unz/pkg/ans"
	"github.com/ha1tch/unz/pkg/bpe"
	"github.com/ha1tch/unz/pkg/detect"
	vocabpkg "github.com/ha1tch/unz/pkg/vocab"
)

// Compression methods (ZIP standard + extensions)
type Method uint16

const (
	MethodStore   Method = 0  // No compression
	MethodDEFLATE Method = 8  // Standard DEFLATE
	MethodUNZLATE Method = 85 // 'U' = BPE + ANS
	MethodBPELATE Method = 86 // 'V' = BPE + DEFLATE (vocabulary-assisted)
)

func (m Method) String() string {
	switch m {
	case MethodStore:
		return "Stored"
	case MethodDEFLATE:
		return "Deflate"
	case MethodUNZLATE:
		return "Unzlate"
	case MethodBPELATE:
		return "Bpelate"
	default:
		return "Unknown"
	}
}

// ZIP signatures
const (
	sigLocalFile   = 0x04034b50
	sigCentralDir  = 0x02014b50
	sigEndCentralD = 0x06054b50
)

// ZIP constants
const (
	zipVersion     = 20     // 2.0 - minimum for DEFLATE
	zipVersionUnix = 0x0314 // Unix, version 2.0
	flagUTF8       = 0x0800 // Bit 11: UTF-8 filename
)

// Unix file type constants (for st_mode)
const (
	unixModeTypeMask = 0170000 // S_IFMT - mask for file type
	unixModeRegular  = 0100000 // S_IFREG - regular file
	unixModeDir      = 0040000 // S_IFDIR - directory
	unixModeSymlink  = 0120000 // S_IFLNK - symbolic link
)

// Extra field IDs
const (
	extraExtendedTS = 0x5455 // Extended timestamp
	extraVocabInfo  = 0x554E // 'UN' - vocabulary/language info for BPELATE
)

// Natural language codes (human languages for comments, docs, strings)
// Ordered by global coverage priority
type NatLang byte

const (
	NatLangUnspecified NatLang = 0x00
	NatLangEnglish     NatLang = 0x01
	NatLangSpanish     NatLang = 0x02
	NatLangFrench      NatLang = 0x03
	NatLangPortuguese  NatLang = 0x04
	NatLangGerman      NatLang = 0x05
	NatLangItalian     NatLang = 0x06
	NatLangDutch       NatLang = 0x07
	NatLangChinese     NatLang = 0x08 // Simplified + Traditional
	NatLangArabic      NatLang = 0x09
	NatLangHindi       NatLang = 0x0A
	NatLangIndonesian  NatLang = 0x0B // Indonesian/Malay
	NatLangBengali     NatLang = 0x0C
	NatLangRussian     NatLang = 0x0D
	NatLangJapanese    NatLang = 0x0E
)

func (n NatLang) String() string {
	names := []string{
		"unspecified", "en", "es", "fr", "pt", "de", "it", "nl",
		"zh", "ar", "hi", "id", "bn", "ru", "ja",
	}
	if int(n) < len(names) {
		return names[n]
	}
	return "unknown"
}

// Programming language codes
type ProgLang byte

const (
	ProgLangNone       ProgLang = 0x00
	ProgLangGo         ProgLang = 0x01
	ProgLangPython     ProgLang = 0x02
	ProgLangJavaScript ProgLang = 0x03 // includes TypeScript
	ProgLangJava       ProgLang = 0x04
	ProgLangC          ProgLang = 0x05
	ProgLangCPP        ProgLang = 0x06
	ProgLangCSharp     ProgLang = 0x07
	ProgLangRuby       ProgLang = 0x08
	ProgLangRust       ProgLang = 0x09
	ProgLangPHP        ProgLang = 0x0A
	ProgLangSwift      ProgLang = 0x0B
	ProgLangKotlin     ProgLang = 0x0C
)

func (p ProgLang) String() string {
	names := []string{
		"none", "go", "python", "javascript", "java", "c", "c++",
		"c#", "ruby", "rust", "php", "swift", "kotlin",
	}
	if int(p) < len(names) {
		return names[p]
	}
	return "unknown"
}

// Structured data format codes
type DataFmt byte

const (
	DataFmtNone DataFmt = 0x00
	DataFmtJSON DataFmt = 0x01
	DataFmtXML  DataFmt = 0x02
	DataFmtYAML DataFmt = 0x03
	DataFmtCSV  DataFmt = 0x04
	DataFmtTOML DataFmt = 0x05
	DataFmtINI  DataFmt = 0x06
)

func (d DataFmt) String() string {
	names := []string{"none", "json", "xml", "yaml", "csv", "toml", "ini"}
	if int(d) < len(names) {
		return names[d]
	}
	return "unknown"
}

// Markup language codes (document formatting)
type MarkupLang byte

const (
	MarkupNone     MarkupLang = 0x00
	MarkupHTML     MarkupLang = 0x01
	MarkupMarkdown MarkupLang = 0x02
	MarkupLaTeX    MarkupLang = 0x03
	MarkupRTF      MarkupLang = 0x04
	MarkupReST     MarkupLang = 0x05 // reStructuredText
	MarkupAsciiDoc MarkupLang = 0x06
	MarkupOrg      MarkupLang = 0x07 // Emacs Org-mode
)

func (m MarkupLang) String() string {
	names := []string{"none", "html", "markdown", "latex", "rtf", "rst", "asciidoc", "org"}
	if int(m) < len(names) {
		return names[m]
	}
	return "unknown"
}

// VocabInfo holds language metadata for BPE vocabulary selection.
// Stored in ZIP extra field 0x554E as 4 bytes.
type VocabInfo struct {
	NatLang  NatLang    // Natural/human language (for comments, docs, strings)
	ProgLang ProgLang   // Programming language
	DataFmt  DataFmt    // Structured data format
	Markup   MarkupLang // Markup/document format
}

// Legacy single-byte language IDs (for backwards compatibility)
const (
	LangIDText byte = 0x00
	LangIDGo   byte = 0x01
	LangIDPy   byte = 0x02
	LangIDJS   byte = 0x03
)

// Errors
var (
	ErrInvalidFormat = errors.New("compress: not a valid ZIP file")
	ErrCorrupted     = errors.New("compress: corrupted data")
	ErrTooShort      = errors.New("compress: data too short")
	ErrUnsupported   = errors.New("compress: unsupported compression method")
	ErrFileTooLarge  = errors.New("compress: file exceeds 4GB limit (ZIP64 not supported)")
)

// FileInfo contains metadata about a file in the archive.
type FileInfo struct {
	Name     string
	Size     int64 // uncompressed size
	CompSize int64 // compressed size
	Method   Method
	CRC32    uint32
	ModTime  time.Time
	Mode     os.FileMode // Unix permissions
	Offset   int64       // offset of local header
	Vocab    VocabInfo   // vocabulary info for BPELATE
}

// Compressor provides ZIP-compatible compression.
type Compressor struct {
	// Default vocabulary (for text)
	encoder *bpe.Encoder
	vocab   *bpe.Vocabulary

	// Language-specific encoders (created on demand)
	goEncoder *bpe.Encoder
	pyEncoder *bpe.Encoder
	jsEncoder *bpe.Encoder
}

// New creates a new compressor with the given BPE vocabulary.
func New(vocab *bpe.Vocabulary) *Compressor {
	return &Compressor{
		encoder: bpe.NewEncoder(vocab),
		vocab:   vocab,
	}
}

// NewWithEncoder creates a compressor with an existing encoder.
func NewWithEncoder(enc *bpe.Encoder) *Compressor {
	return &Compressor{
		encoder: enc,
		vocab:   enc.Vocabulary(),
	}
}

// Archive builds a multi-file ZIP archive.
type Archive struct {
	compressor *Compressor
	entries    []archiveEntry
}

type archiveEntry struct {
	name       string
	data       []byte
	compressed []byte
	method     Method
	crc        uint32
	modTime    time.Time
	mode       os.FileMode
	vocabInfo  VocabInfo
	isSymlink  bool   // true if this is a symbolic link
	linkTarget string // target path for symlinks
}

// NewArchive creates a new archive builder.
func NewArchive(c *Compressor) *Archive {
	return &Archive{compressor: c}
}

// Add adds a file to the archive with automatic method selection.
func (a *Archive) Add(data []byte, name string, modTime time.Time, mode os.FileMode) error {
	if len(data) > 0xFFFFFFFF {
		return ErrFileTooLarge
	}

	entry := archiveEntry{
		name:    name,
		data:    data,
		crc:     crc32.ChecksumIEEE(data),
		modTime: modTime,
		mode:    mode,
	}

	if len(data) == 0 {
		entry.method = MethodStore
		entry.compressed = data
	} else {
		profile := detect.Detect(data)

		switch profile.Type {
		case detect.TypeText:
			compressed, method, vocab := a.compressor.compressTextBest(data)
			entry.compressed = compressed
			entry.method = method
			entry.vocabInfo = vocab
		case detect.TypeCode:
			compressed, method, vocab := a.compressor.compressCodeBest(data, profile.Language)
			entry.compressed = compressed
			entry.method = method
			entry.vocabInfo = vocab
		case detect.TypeRandom:
			entry.method = MethodStore
			entry.compressed = data
		default:
			compressed, _ := a.compressor.compressDEFLATE(data)
			entry.method = MethodDEFLATE
			entry.compressed = compressed
		}
	}

	if len(entry.compressed) > 0xFFFFFFFF {
		return ErrFileTooLarge
	}

	a.entries = append(a.entries, entry)
	return nil
}

// AddStore adds a file to the archive without compression (store only).
func (a *Archive) AddStore(data []byte, name string, modTime time.Time, mode os.FileMode) error {
	if len(data) > 0xFFFFFFFF {
		return ErrFileTooLarge
	}

	entry := archiveEntry{
		name:       name,
		data:       data,
		compressed: data,
		method:     MethodStore,
		crc:        crc32.ChecksumIEEE(data),
		modTime:    modTime,
		mode:       mode,
	}

	a.entries = append(a.entries, entry)
	return nil
}

// AddDirectory adds a directory entry to the archive.
func (a *Archive) AddDirectory(name string, modTime time.Time, mode os.FileMode) error {
	// Ensure directory name ends with /
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}

	entry := archiveEntry{
		name:       name,
		data:       nil,
		compressed: nil,
		method:     MethodStore,
		crc:        0,
		modTime:    modTime,
		mode:       mode | os.ModeDir,
	}

	a.entries = append(a.entries, entry)
	return nil
}

// AddSymlink adds a symbolic link entry to the archive.
// The link target is stored as the file content.
func (a *Archive) AddSymlink(name string, target string, modTime time.Time, mode os.FileMode) error {
	targetBytes := []byte(target)

	entry := archiveEntry{
		name:       name,
		data:       targetBytes,
		compressed: targetBytes,
		method:     MethodStore,
		crc:        crc32.ChecksumIEEE(targetBytes),
		modTime:    modTime,
		mode:       mode | os.ModeSymlink,
		isSymlink:  true,
		linkTarget: target,
	}

	a.entries = append(a.entries, entry)
	return nil
}

// Bytes returns the complete ZIP archive.
func (a *Archive) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	var centralDir bytes.Buffer

	for _, entry := range a.entries {
		dosTime, dosDate := timeToDOS(entry.modTime)
		flags := uint16(0)
		if hasNonASCII(entry.name) {
			flags |= flagUTF8
		}

		// Build extra fields
		extraLocal := makeExtendedTimestamp(entry.modTime, true)
		extraCentral := makeExtendedTimestamp(entry.modTime, false)

		// Add vocabulary info for BPELATE
		if entry.method == MethodBPELATE {
			vocabExtra := makeVocabInfo(entry.vocabInfo)
			extraLocal = append(extraLocal, vocabExtra...)
			extraCentral = append(extraCentral, vocabExtra...)
		}

		// Unix external attributes (convert Go mode to Unix st_mode)
		externalAttrs := goModeToUnix(entry.mode) << 16

		// Local file header
		localHeaderOffset := buf.Len()
		writeLocalHeader(&buf, entry.name, entry.method, flags, dosTime, dosDate, entry.crc,
			uint32(len(entry.compressed)), uint32(len(entry.data)), extraLocal)

		// File data
		buf.Write(entry.compressed)

		// Central directory entry
		writeCentralDir(&centralDir, entry.name, entry.method, flags, dosTime, dosDate, entry.crc,
			uint32(len(entry.compressed)), uint32(len(entry.data)), uint32(localHeaderOffset),
			externalAttrs, extraCentral)
	}

	// Append central directory
	centralDirOffset := buf.Len()
	buf.Write(centralDir.Bytes())
	centralDirSize := buf.Len() - centralDirOffset

	// End of central directory
	writeEndCentralDir(&buf, len(a.entries), uint32(centralDirSize), uint32(centralDirOffset))

	return buf.Bytes(), nil
}

// compressTextBest compresses text and returns best result with method and vocab info.
func (c *Compressor) compressTextBest(data []byte) ([]byte, Method, VocabInfo) {
	deflateData, _ := c.compressDEFLATE(data)
	bpelateData, _ := c.compressBPELATE(data)

	vocab := VocabInfo{NatLang: NatLangEnglish}

	if len(bpelateData) < len(deflateData) {
		return bpelateData, MethodBPELATE, vocab
	}
	return deflateData, MethodDEFLATE, vocab
}

// compressCodeBest compresses code and returns best result with method and vocab info.
func (c *Compressor) compressCodeBest(data []byte, lang detect.CodeLang) ([]byte, Method, VocabInfo) {
	encoder := c.getEncoderForLang(lang)
	vocabInfo := makeVocabInfoFromDetect(lang)

	deflateData, _ := c.compressDEFLATE(data)
	bpelateData, _ := c.compressBPELATEWith(data, encoder)

	if len(bpelateData) < len(deflateData) {
		return bpelateData, MethodBPELATE, vocabInfo
	}
	return deflateData, MethodDEFLATE, vocabInfo
}

// CompressFile creates a ZIP archive containing one file.
func (c *Compressor) CompressFile(data []byte, name string, modTime time.Time) ([]byte, error) {
	return c.CompressFileWithMode(data, name, modTime, 0644)
}

// CompressFileWithMode creates a ZIP archive with specified Unix permissions.
func (c *Compressor) CompressFileWithMode(data []byte, name string, modTime time.Time, mode os.FileMode) ([]byte, error) {
	if len(data) == 0 {
		return c.createZIP(data, name, modTime, mode, MethodStore)
	}

	profile := detect.Detect(data)

	switch profile.Type {
	case detect.TypeText:
		// Use BPELATE with text vocabulary - compare against DEFLATE
		return c.compressText(data, name, modTime, mode)
	case detect.TypeCode:
		// Try language-specific UNZLATE and compare with DEFLATE
		return c.compressCode(data, name, modTime, mode, profile.Language)
	case detect.TypeRandom:
		return c.createZIP(data, name, modTime, mode, MethodStore)
	default:
		return c.createZIP(data, name, modTime, mode, MethodDEFLATE)
	}
}

// compressCode compresses source code using the best method.
// It tries DEFLATE, UNZLATE (BPE+ANS), and BPELATE (BPE+DEFLATE),
// then picks whichever produces the smallest output.
func (c *Compressor) compressCode(data []byte, name string, modTime time.Time, mode os.FileMode, lang detect.CodeLang) ([]byte, error) {
	// Get language-specific encoder
	encoder := c.getEncoderForLang(lang)

	// Try all three methods
	deflateData, deflateErr := c.compressDEFLATE(data)
	unzlateData, unzlateErr := c.compressUNZLATEWith(data, encoder)
	bpelateData, bpelateErr := c.compressBPELATEWith(data, encoder)

	// Find the smallest successful result
	type candidate struct {
		data   []byte
		method Method
		err    error
	}

	candidates := []candidate{
		{deflateData, MethodDEFLATE, deflateErr},
		{unzlateData, MethodUNZLATE, unzlateErr},
		{bpelateData, MethodBPELATE, bpelateErr},
	}

	var best *candidate
	for i := range candidates {
		c := &candidates[i]
		if c.err != nil {
			continue
		}
		if best == nil || len(c.data) < len(best.data) {
			best = c
		}
	}

	if best == nil {
		// All failed - return first error
		if deflateErr != nil {
			return nil, deflateErr
		}
		return nil, unzlateErr
	}

	// For BPELATE, include language info in metadata
	if best.method == MethodBPELATE {
		vocabInfo := makeVocabInfoFromDetect(lang)
		return c.createZIPWithCompressedAndLang(data, best.data, name, modTime, mode, best.method, vocabInfo)
	}

	return c.createZIPWithCompressed(data, best.data, name, modTime, mode, best.method)
}

// compressText compresses natural language text using the best method.
// It compares DEFLATE and BPELATE (with text vocabulary) and picks the smaller result.
func (c *Compressor) compressText(data []byte, name string, modTime time.Time, mode os.FileMode) ([]byte, error) {
	// Try DEFLATE and BPELATE with text vocabulary
	deflateData, deflateErr := c.compressDEFLATE(data)
	bpelateData, bpelateErr := c.compressBPELATEWith(data, c.encoder) // default encoder uses text vocab

	// Pick the smaller result
	if deflateErr != nil && bpelateErr != nil {
		return nil, deflateErr
	}

	if bpelateErr != nil || (deflateErr == nil && len(deflateData) <= len(bpelateData)) {
		return c.createZIPWithCompressed(data, deflateData, name, modTime, mode, MethodDEFLATE)
	}

	return c.createZIPWithCompressed(data, bpelateData, name, modTime, mode, MethodBPELATE)
}

// makeVocabInfoFromDetect creates VocabInfo from detected language.
func makeVocabInfoFromDetect(lang detect.CodeLang) VocabInfo {
	info := VocabInfo{NatLang: NatLangEnglish} // assume English comments
	switch lang {
	case detect.CodeLangGo:
		info.ProgLang = ProgLangGo
	case detect.CodeLangPython:
		info.ProgLang = ProgLangPython
	case detect.CodeLangJavaScript:
		info.ProgLang = ProgLangJavaScript
	}
	return info
}

// getEncoderForLang returns the appropriate encoder for the language.
func (c *Compressor) getEncoderForLang(lang detect.CodeLang) *bpe.Encoder {
	switch lang {
	case detect.CodeLangGo:
		if c.goEncoder == nil {
			c.goEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangGo))
		}
		return c.goEncoder
	case detect.CodeLangPython:
		if c.pyEncoder == nil {
			c.pyEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangPython))
		}
		return c.pyEncoder
	case detect.CodeLangJavaScript:
		if c.jsEncoder == nil {
			c.jsEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangJavaScript))
		}
		return c.jsEncoder
	default:
		return c.encoder
	}
}

// CompressFileAs creates a ZIP archive using a specific method.
func (c *Compressor) CompressFileAs(data []byte, name string, modTime time.Time, method Method) ([]byte, error) {
	return c.createZIP(data, name, modTime, 0644, method)
}

// CompressFileAsWithMode creates a ZIP archive using a specific method and permissions.
func (c *Compressor) CompressFileAsWithMode(data []byte, name string, modTime time.Time, mode os.FileMode, method Method) ([]byte, error) {
	return c.createZIP(data, name, modTime, mode, method)
}

// createZIP builds a complete ZIP archive.
func (c *Compressor) createZIP(data []byte, name string, modTime time.Time, mode os.FileMode, method Method) ([]byte, error) {
	// Check size limits (no ZIP64 support)
	if len(data) > 0xFFFFFFFF {
		return nil, ErrFileTooLarge
	}

	// Compress data
	var compressed []byte
	var err error

	switch method {
	case MethodUNZLATE:
		compressed, err = c.compressUNZLATE(data)
	case MethodBPELATE:
		compressed, err = c.compressBPELATE(data)
	case MethodDEFLATE:
		compressed, err = c.compressDEFLATE(data)
	case MethodStore:
		compressed = data
	default:
		return nil, ErrUnsupported
	}

	if err != nil {
		return nil, err
	}

	if len(compressed) > 0xFFFFFFFF {
		return nil, ErrFileTooLarge
	}

	crc := crc32.ChecksumIEEE(data)
	dosTime, dosDate := timeToDOS(modTime)
	flags := uint16(0)

	// Set UTF-8 flag if filename contains non-ASCII
	if hasNonASCII(name) {
		flags |= flagUTF8
	}

	// Build extended timestamp extra field
	extraLocal := makeExtendedTimestamp(modTime, true)
	extraCentral := makeExtendedTimestamp(modTime, false)

	// Unix external attributes: convert Go mode to Unix st_mode
	externalAttrs := goModeToUnix(mode) << 16

	// Build archive
	var buf bytes.Buffer

	// Local file header
	localHeaderOffset := buf.Len()
	writeLocalHeader(&buf, name, method, flags, dosTime, dosDate, crc,
		uint32(len(compressed)), uint32(len(data)), extraLocal)

	// File data
	buf.Write(compressed)

	// Central directory
	centralDirOffset := buf.Len()
	writeCentralDir(&buf, name, method, flags, dosTime, dosDate, crc,
		uint32(len(compressed)), uint32(len(data)), uint32(localHeaderOffset),
		externalAttrs, extraCentral)
	centralDirSize := buf.Len() - centralDirOffset

	// End of central directory
	writeEndCentralDir(&buf, 1, uint32(centralDirSize), uint32(centralDirOffset))

	return buf.Bytes(), nil
}

// createZIPWithCompressed builds a ZIP archive with pre-compressed data.
func (c *Compressor) createZIPWithCompressed(originalData, compressed []byte, name string, modTime time.Time, mode os.FileMode, method Method) ([]byte, error) {
	return c.createZIPWithCompressedAndLang(originalData, compressed, name, modTime, mode, method, VocabInfo{})
}

// createZIPWithCompressedAndLang builds a ZIP archive with pre-compressed data and language info.
func (c *Compressor) createZIPWithCompressedAndLang(originalData, compressed []byte, name string, modTime time.Time, mode os.FileMode, method Method, vocabInfo VocabInfo) ([]byte, error) {
	// Check size limits (no ZIP64 support)
	if len(originalData) > 0xFFFFFFFF || len(compressed) > 0xFFFFFFFF {
		return nil, ErrFileTooLarge
	}

	crc := crc32.ChecksumIEEE(originalData)
	dosTime, dosDate := timeToDOS(modTime)
	flags := uint16(0)

	// Set UTF-8 flag if filename contains non-ASCII
	if hasNonASCII(name) {
		flags |= flagUTF8
	}

	// Build extra fields
	extraLocal := makeExtendedTimestamp(modTime, true)
	extraCentral := makeExtendedTimestamp(modTime, false)

	// Add vocabulary info for BPELATE
	if method == MethodBPELATE {
		vocabExtra := makeVocabInfo(vocabInfo)
		extraLocal = append(extraLocal, vocabExtra...)
		extraCentral = append(extraCentral, vocabExtra...)
	}

	// Unix external attributes: convert Go mode to Unix st_mode
	externalAttrs := goModeToUnix(mode) << 16

	// Build archive
	var buf bytes.Buffer

	// Local file header
	localHeaderOffset := buf.Len()
	writeLocalHeader(&buf, name, method, flags, dosTime, dosDate, crc,
		uint32(len(compressed)), uint32(len(originalData)), extraLocal)

	// File data
	buf.Write(compressed)

	// Central directory
	centralDirOffset := buf.Len()
	writeCentralDir(&buf, name, method, flags, dosTime, dosDate, crc,
		uint32(len(compressed)), uint32(len(originalData)), uint32(localHeaderOffset),
		externalAttrs, extraCentral)
	centralDirSize := buf.Len() - centralDirOffset

	// End of central directory
	writeEndCentralDir(&buf, 1, uint32(centralDirSize), uint32(centralDirOffset))

	return buf.Bytes(), nil
}

// makeVocabInfo creates the vocabulary info extra field (0x554E).
// Format: 2 bytes ID + 2 bytes size + 4 bytes data (NatLang, ProgLang, DataFmt, Markup)
func makeVocabInfo(info VocabInfo) []byte {
	extra := make([]byte, 8)
	binary.LittleEndian.PutUint16(extra[0:2], extraVocabInfo)
	binary.LittleEndian.PutUint16(extra[2:4], 4) // size: 4 bytes
	extra[4] = byte(info.NatLang)
	extra[5] = byte(info.ProgLang)
	extra[6] = byte(info.DataFmt)
	extra[7] = byte(info.Markup)
	return extra
}

// makeExtendedTimestamp creates the 0x5455 extra field.
// Local header includes mtime; central directory is shorter (just flags + mtime).
func makeExtendedTimestamp(t time.Time, local bool) []byte {
	if t.IsZero() {
		return nil
	}

	// Extra field: 2 bytes ID + 2 bytes size + 1 byte flags + 4 bytes mtime
	var extra bytes.Buffer
	binary.Write(&extra, binary.LittleEndian, uint16(extraExtendedTS))

	if local {
		binary.Write(&extra, binary.LittleEndian, uint16(5)) // size: flags(1) + mtime(4)
	} else {
		binary.Write(&extra, binary.LittleEndian, uint16(5)) // same for central
	}

	extra.WriteByte(0x01) // flags: bit 0 = mtime present
	binary.Write(&extra, binary.LittleEndian, uint32(t.Unix()))

	return extra.Bytes()
}

// hasNonASCII returns true if the string contains any non-ASCII bytes.
func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return true
		}
	}
	return false
}

// writeLocalHeader writes a ZIP local file header.
func writeLocalHeader(w *bytes.Buffer, name string, method Method, flags, dosTime, dosDate uint16, crc, compSize, uncompSize uint32, extra []byte) {
	var hdr [30]byte
	binary.LittleEndian.PutUint32(hdr[0:4], sigLocalFile)
	binary.LittleEndian.PutUint16(hdr[4:6], zipVersion)
	binary.LittleEndian.PutUint16(hdr[6:8], flags)
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(method))
	binary.LittleEndian.PutUint16(hdr[10:12], dosTime)
	binary.LittleEndian.PutUint16(hdr[12:14], dosDate)
	binary.LittleEndian.PutUint32(hdr[14:18], crc)
	binary.LittleEndian.PutUint32(hdr[18:22], compSize)
	binary.LittleEndian.PutUint32(hdr[22:26], uncompSize)
	binary.LittleEndian.PutUint16(hdr[26:28], uint16(len(name)))
	binary.LittleEndian.PutUint16(hdr[28:30], uint16(len(extra)))

	w.Write(hdr[:])
	w.WriteString(name)
	w.Write(extra)
}

// writeCentralDir writes a ZIP central directory entry.
func writeCentralDir(w *bytes.Buffer, name string, method Method, flags, dosTime, dosDate uint16, crc, compSize, uncompSize, localOffset, externalAttrs uint32, extra []byte) {
	var hdr [46]byte
	binary.LittleEndian.PutUint32(hdr[0:4], sigCentralDir)
	binary.LittleEndian.PutUint16(hdr[4:6], zipVersionUnix) // version made by (Unix)
	binary.LittleEndian.PutUint16(hdr[6:8], zipVersion)     // version needed
	binary.LittleEndian.PutUint16(hdr[8:10], flags)
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(method))
	binary.LittleEndian.PutUint16(hdr[12:14], dosTime)
	binary.LittleEndian.PutUint16(hdr[14:16], dosDate)
	binary.LittleEndian.PutUint32(hdr[16:20], crc)
	binary.LittleEndian.PutUint32(hdr[20:24], compSize)
	binary.LittleEndian.PutUint32(hdr[24:28], uncompSize)
	binary.LittleEndian.PutUint16(hdr[28:30], uint16(len(name)))
	binary.LittleEndian.PutUint16(hdr[30:32], uint16(len(extra)))
	binary.LittleEndian.PutUint16(hdr[32:34], 0) // comment length
	binary.LittleEndian.PutUint16(hdr[34:36], 0) // disk number
	binary.LittleEndian.PutUint16(hdr[36:38], 0) // internal attrs
	binary.LittleEndian.PutUint32(hdr[38:42], externalAttrs)
	binary.LittleEndian.PutUint32(hdr[42:46], localOffset)

	w.Write(hdr[:])
	w.WriteString(name)
	w.Write(extra)
}

// writeEndCentralDir writes the ZIP end of central directory record.
func writeEndCentralDir(w *bytes.Buffer, numEntries int, centralDirSize, centralDirOffset uint32) {
	var hdr [22]byte
	binary.LittleEndian.PutUint32(hdr[0:4], sigEndCentralD)
	binary.LittleEndian.PutUint16(hdr[4:6], 0)                    // disk number
	binary.LittleEndian.PutUint16(hdr[6:8], 0)                    // disk with central dir
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(numEntries))  // entries on disk
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(numEntries)) // total entries
	binary.LittleEndian.PutUint32(hdr[12:16], centralDirSize)
	binary.LittleEndian.PutUint32(hdr[16:20], centralDirOffset)
	binary.LittleEndian.PutUint16(hdr[20:22], 0) // comment length

	w.Write(hdr[:])
}

// Decompress extracts the first file from a ZIP archive.
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	info, err := GetFileInfo(data)
	if err != nil {
		return nil, err
	}

	// Find file data (after local header + name + extra)
	nameLen := binary.LittleEndian.Uint16(data[26:28])
	extraLen := binary.LittleEndian.Uint16(data[28:30])
	dataOffset := 30 + int(nameLen) + int(extraLen)

	if len(data) < dataOffset+int(info.CompSize) {
		return nil, ErrCorrupted
	}

	compressed := data[dataOffset : dataOffset+int(info.CompSize)]

	switch info.Method {
	case MethodUNZLATE:
		return c.decompressUNZLATE(compressed)
	case MethodBPELATE:
		return c.decompressBPELATEWithVocab(compressed, info.Vocab)
	case MethodDEFLATE:
		return c.decompressDEFLATE(compressed)
	case MethodStore:
		return compressed, nil
	default:
		return nil, ErrUnsupported
	}
}

// ListFiles returns metadata for all files in a ZIP archive.
func ListFiles(data []byte) ([]*FileInfo, error) {
	if len(data) < 22 {
		return nil, ErrTooShort
	}

	// Find end of central directory
	eocdOffset := -1
	for i := len(data) - 22; i >= 0 && i > len(data)-65557; i-- {
		if binary.LittleEndian.Uint32(data[i:i+4]) == sigEndCentralD {
			eocdOffset = i
			break
		}
	}
	if eocdOffset < 0 {
		return nil, ErrInvalidFormat
	}

	// Parse EOCD
	numEntries := int(binary.LittleEndian.Uint16(data[eocdOffset+10 : eocdOffset+12]))
	centralDirOffset := int(binary.LittleEndian.Uint32(data[eocdOffset+16 : eocdOffset+20]))

	if centralDirOffset >= len(data) {
		return nil, ErrCorrupted
	}

	// Parse central directory entries
	var files []*FileInfo
	offset := centralDirOffset

	for i := 0; i < numEntries && offset < eocdOffset; i++ {
		if offset+46 > len(data) {
			break
		}

		sig := binary.LittleEndian.Uint32(data[offset : offset+4])
		if sig != sigCentralDir {
			break
		}

		method := Method(binary.LittleEndian.Uint16(data[offset+10 : offset+12]))
		dosTime := binary.LittleEndian.Uint16(data[offset+12 : offset+14])
		dosDate := binary.LittleEndian.Uint16(data[offset+14 : offset+16])
		crc := binary.LittleEndian.Uint32(data[offset+16 : offset+20])
		compSize := binary.LittleEndian.Uint32(data[offset+20 : offset+24])
		uncompSize := binary.LittleEndian.Uint32(data[offset+24 : offset+28])
		nameLen := int(binary.LittleEndian.Uint16(data[offset+28 : offset+30]))
		extraLen := int(binary.LittleEndian.Uint16(data[offset+30 : offset+32]))
		commentLen := int(binary.LittleEndian.Uint16(data[offset+32 : offset+34]))
		externalAttrs := binary.LittleEndian.Uint32(data[offset+38 : offset+42])
		localOffset := binary.LittleEndian.Uint32(data[offset+42 : offset+46])

		if offset+46+nameLen+extraLen+commentLen > len(data) {
			break
		}

		name := string(data[offset+46 : offset+46+nameLen])

		// Parse modification time
		modTime := dosToTime(dosTime, dosDate)
		vocab := VocabInfo{}

		// Parse extra fields
		if extraLen > 0 {
			extra := data[offset+46+nameLen : offset+46+nameLen+extraLen]
			if unixTime, ok := parseExtendedTimestamp(extra); ok {
				modTime = unixTime
			}
			if parsedVocab, ok := parseVocabInfo(extra); ok {
				vocab = parsedVocab
			}
		}

		// Unix mode from external attributes
		mode := os.FileMode(0644)
		versionMadeBy := binary.LittleEndian.Uint16(data[offset+4 : offset+6])
		if (versionMadeBy >> 8) == 3 { // Unix
			mode = unixModeToGo(externalAttrs >> 16)
		}

		// Check if directory (fallback for non-Unix archives)
		if strings.HasSuffix(name, "/") && mode&os.ModeDir == 0 {
			mode |= os.ModeDir
		}

		files = append(files, &FileInfo{
			Name:     name,
			Size:     int64(uncompSize),
			CompSize: int64(compSize),
			Method:   method,
			CRC32:    crc,
			ModTime:  modTime,
			Mode:     mode,
			Offset:   int64(localOffset),
			Vocab:    vocab,
		})

		offset += 46 + nameLen + extraLen + commentLen
	}

	return files, nil
}

// DecompressFile extracts a specific file from a ZIP archive by its FileInfo.
func (c *Compressor) DecompressFile(data []byte, info *FileInfo) ([]byte, error) {
	offset := int(info.Offset)

	if offset+30 > len(data) {
		return nil, ErrCorrupted
	}

	// Verify local header signature
	sig := binary.LittleEndian.Uint32(data[offset : offset+4])
	if sig != sigLocalFile {
		return nil, ErrCorrupted
	}

	// Get local header name and extra lengths (may differ from central dir)
	nameLen := int(binary.LittleEndian.Uint16(data[offset+26 : offset+28]))
	extraLen := int(binary.LittleEndian.Uint16(data[offset+28 : offset+30]))

	dataOffset := offset + 30 + nameLen + extraLen

	if dataOffset+int(info.CompSize) > len(data) {
		return nil, ErrCorrupted
	}

	// Directory entries have no data
	if strings.HasSuffix(info.Name, "/") || info.Size == 0 && info.CompSize == 0 {
		return nil, nil
	}

	compressed := data[dataOffset : dataOffset+int(info.CompSize)]

	switch info.Method {
	case MethodUNZLATE:
		return c.decompressUNZLATE(compressed)
	case MethodBPELATE:
		return c.decompressBPELATEWithVocab(compressed, info.Vocab)
	case MethodDEFLATE:
		return c.decompressDEFLATE(compressed)
	case MethodStore:
		return compressed, nil
	default:
		return nil, ErrUnsupported
	}
}

// DecompressAll extracts all files from a ZIP archive.
// Returns a map of filename to file contents.
func (c *Compressor) DecompressAll(data []byte) (map[string][]byte, error) {
	files, err := ListFiles(data)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte)
	for _, info := range files {
		// Skip directories
		if strings.HasSuffix(info.Name, "/") {
			continue
		}

		content, err := c.DecompressFile(data, info)
		if err != nil {
			return nil, err
		}
		result[info.Name] = content
	}

	return result, nil
}

// compressUNZLATE compresses using BPE + ANS.
func (c *Compressor) compressUNZLATE(data []byte) ([]byte, error) {
	tokens := c.encoder.Encode(data)
	if len(tokens) == 0 {
		return data, nil
	}

	tokenBytes := encodeVarints(tokens)
	return ans.Compress(tokenBytes)
}

// compressUNZLATEWith compresses using BPE + ANS with a specific encoder.
func (c *Compressor) compressUNZLATEWith(data []byte, encoder *bpe.Encoder) ([]byte, error) {
	tokens := encoder.Encode(data)
	if len(tokens) == 0 {
		return data, nil
	}

	tokenBytes := encodeVarints(tokens)
	return ans.Compress(tokenBytes)
}

// decompressUNZLATE decompresses BPE + ANS data.
func (c *Compressor) decompressUNZLATE(data []byte) ([]byte, error) {
	tokenBytes, err := ans.Decompress(data)
	if err != nil {
		return nil, err
	}

	tokens := decodeVarints(tokenBytes)
	return c.encoder.Decode(tokens), nil
}

// compressBPELATE compresses using BPE + DEFLATE.
func (c *Compressor) compressBPELATE(data []byte) ([]byte, error) {
	tokens := c.encoder.Encode(data)
	if len(tokens) == 0 {
		return c.compressDEFLATE(data)
	}

	tokenBytes := encodeVarints(tokens)
	return c.compressDEFLATE(tokenBytes)
}

// compressBPELATEWith compresses using BPE + DEFLATE with a specific encoder.
func (c *Compressor) compressBPELATEWith(data []byte, encoder *bpe.Encoder) ([]byte, error) {
	tokens := encoder.Encode(data)
	if len(tokens) == 0 {
		return c.compressDEFLATE(data)
	}

	tokenBytes := encodeVarints(tokens)
	return c.compressDEFLATE(tokenBytes)
}

// decompressBPELATE decompresses BPE + DEFLATE data using default encoder.
func (c *Compressor) decompressBPELATE(data []byte) ([]byte, error) {
	return c.decompressBPELATEWithVocab(data, VocabInfo{})
}

// decompressBPELATEWithVocab decompresses BPE + DEFLATE data with specified vocabulary info.
func (c *Compressor) decompressBPELATEWithVocab(data []byte, vocab VocabInfo) ([]byte, error) {
	tokenBytes, err := c.decompressDEFLATE(data)
	if err != nil {
		return nil, err
	}

	if len(tokenBytes) == 0 {
		return tokenBytes, nil
	}

	encoder := c.getEncoderForProgLang(vocab.ProgLang)
	tokens := decodeVarints(tokenBytes)
	return encoder.Decode(tokens), nil
}

// getEncoderForProgLang returns the encoder for a programming language.
func (c *Compressor) getEncoderForProgLang(lang ProgLang) *bpe.Encoder {
	switch lang {
	case ProgLangGo:
		if c.goEncoder == nil {
			c.goEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangGo))
		}
		return c.goEncoder
	case ProgLangPython:
		if c.pyEncoder == nil {
			c.pyEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangPython))
		}
		return c.pyEncoder
	case ProgLangJavaScript:
		if c.jsEncoder == nil {
			c.jsEncoder = bpe.NewEncoder(vocabpkg.ForLanguage(vocabpkg.LangJavaScript))
		}
		return c.jsEncoder
	default:
		return c.encoder
	}
}

// compressDEFLATE compresses using DEFLATE.
func (c *Compressor) compressDEFLATE(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	w.Write(data)
	w.Close()
	return buf.Bytes(), nil
}

// decompressDEFLATE decompresses DEFLATE data.
func (c *Compressor) decompressDEFLATE(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	return io.ReadAll(r)
}

// encodeVarints encodes integers as variable-length bytes.
func encodeVarints(values []int) []byte {
	buf := make([]byte, len(values)*5)
	pos := 0
	for _, v := range values {
		for v >= 0x80 {
			buf[pos] = byte(v) | 0x80
			v >>= 7
			pos++
		}
		buf[pos] = byte(v)
		pos++
	}
	return buf[:pos]
}

// decodeVarints decodes variable-length integers.
func decodeVarints(data []byte) []int {
	values := make([]int, 0, len(data)/2)
	pos := 0
	for pos < len(data) {
		v := 0
		shift := 0
		for pos < len(data) {
			b := data[pos]
			pos++
			v |= int(b&0x7F) << shift
			if b < 0x80 {
				break
			}
			shift += 7
		}
		values = append(values, v)
	}
	return values
}

// timeToDOS converts time.Time to DOS date/time format.
func timeToDOS(t time.Time) (dosTime, dosDate uint16) {
	if t.IsZero() {
		return 0, 0
	}
	// Clamp to DOS valid range (1980-2107)
	year := t.Year()
	if year < 1980 {
		year = 1980
	} else if year > 2107 {
		year = 2107
	}
	dosTime = uint16(t.Second()/2) | uint16(t.Minute())<<5 | uint16(t.Hour())<<11
	dosDate = uint16(t.Day()) | uint16(t.Month())<<5 | uint16(year-1980)<<9
	return
}

// dosToTime converts DOS date/time to time.Time.
func dosToTime(dosTime, dosDate uint16) time.Time {
	if dosTime == 0 && dosDate == 0 {
		return time.Time{}
	}
	sec := int(dosTime&0x1F) * 2
	min := int((dosTime >> 5) & 0x3F)
	hour := int(dosTime >> 11)
	day := int(dosDate & 0x1F)
	month := time.Month((dosDate >> 5) & 0x0F)
	year := int(dosDate>>9) + 1980
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// === Utility functions ===

// GetFileInfo extracts metadata from the first file in a ZIP archive.
func GetFileInfo(data []byte) (*FileInfo, error) {
	if len(data) < 30 {
		return nil, ErrTooShort
	}

	// Check local file header signature
	sig := binary.LittleEndian.Uint32(data[0:4])
	if sig != sigLocalFile {
		return nil, ErrInvalidFormat
	}

	flags := binary.LittleEndian.Uint16(data[6:8])
	method := Method(binary.LittleEndian.Uint16(data[8:10]))
	dosTime := binary.LittleEndian.Uint16(data[10:12])
	dosDate := binary.LittleEndian.Uint16(data[12:14])
	crc := binary.LittleEndian.Uint32(data[14:18])
	compSize := binary.LittleEndian.Uint32(data[18:22])
	uncompSize := binary.LittleEndian.Uint32(data[22:26])
	nameLen := binary.LittleEndian.Uint16(data[26:28])
	extraLen := binary.LittleEndian.Uint16(data[28:30])

	if len(data) < 30+int(nameLen)+int(extraLen) {
		return nil, ErrCorrupted
	}

	name := string(data[30 : 30+nameLen])

	// Parse modification time (prefer extended timestamp if present)
	modTime := dosToTime(dosTime, dosDate)
	vocab := VocabInfo{} // default

	// Parse extra fields
	if extraLen > 0 {
		extra := data[30+nameLen : 30+int(nameLen)+int(extraLen)]
		if unixTime, ok := parseExtendedTimestamp(extra); ok {
			modTime = unixTime
		}
		if parsedVocab, ok := parseVocabInfo(extra); ok {
			vocab = parsedVocab
		}
	}

	// Parse Unix mode from central directory (if we can find it)
	mode := os.FileMode(0644) // default
	if cdOffset := findCentralDirectory(data); cdOffset >= 0 {
		if m, ok := parseUnixMode(data[cdOffset:]); ok {
			mode = m
		}
	}

	_ = flags // UTF-8 flag doesn't affect decoding in Go

	return &FileInfo{
		Name:     name,
		Size:     int64(uncompSize),
		CompSize: int64(compSize),
		Method:   method,
		CRC32:    crc,
		ModTime:  modTime,
		Mode:     mode,
		Offset:   0,
		Vocab:    vocab,
	}, nil
}

// parseExtendedTimestamp extracts Unix mtime from extra field 0x5455.
func parseExtendedTimestamp(extra []byte) (time.Time, bool) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra[0:2])
		size := binary.LittleEndian.Uint16(extra[2:4])

		if len(extra) < 4+int(size) {
			break
		}

		if id == extraExtendedTS && size >= 5 {
			flags := extra[4]
			if flags&0x01 != 0 { // mtime present
				mtime := binary.LittleEndian.Uint32(extra[5:9])
				return time.Unix(int64(mtime), 0), true
			}
		}

		extra = extra[4+size:]
	}
	return time.Time{}, false
}

// parseVocabInfo extracts vocabulary info from extra field 0x554E.
// Supports both old 1-byte format (legacy) and new 4-byte format.
func parseVocabInfo(extra []byte) (VocabInfo, bool) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra[0:2])
		size := binary.LittleEndian.Uint16(extra[2:4])

		if len(extra) < 4+int(size) {
			break
		}

		if id == extraVocabInfo {
			if size >= 4 {
				// New 4-byte format
				return VocabInfo{
					NatLang:  NatLang(extra[4]),
					ProgLang: ProgLang(extra[5]),
					DataFmt:  DataFmt(extra[6]),
					Markup:   MarkupLang(extra[7]),
				}, true
			} else if size >= 1 {
				// Legacy 1-byte format: map old langID to ProgLang
				legacyID := extra[4]
				info := VocabInfo{NatLang: NatLangEnglish} // assume English
				switch legacyID {
				case LangIDGo:
					info.ProgLang = ProgLangGo
				case LangIDPy:
					info.ProgLang = ProgLangPython
				case LangIDJS:
					info.ProgLang = ProgLangJavaScript
				}
				return info, true
			}
		}

		extra = extra[4+size:]
	}
	return VocabInfo{}, false
}

// findCentralDirectory locates the central directory in the archive.
func findCentralDirectory(data []byte) int {
	// Search backwards for EOCD signature
	for i := len(data) - 22; i >= 0 && i > len(data)-65557; i-- {
		if binary.LittleEndian.Uint32(data[i:i+4]) == sigEndCentralD {
			// Found EOCD, get central directory offset
			if len(data) >= i+20 {
				return int(binary.LittleEndian.Uint32(data[i+16 : i+20]))
			}
		}
	}
	return -1
}

// parseUnixMode extracts Unix permissions from central directory external attrs.
func parseUnixMode(cd []byte) (os.FileMode, bool) {
	if len(cd) < 46 {
		return 0, false
	}

	sig := binary.LittleEndian.Uint32(cd[0:4])
	if sig != sigCentralDir {
		return 0, false
	}

	versionMadeBy := binary.LittleEndian.Uint16(cd[4:6])
	// Check if made by Unix (upper byte = 3)
	if (versionMadeBy >> 8) != 3 {
		return 0, false
	}

	externalAttrs := binary.LittleEndian.Uint32(cd[38:42])
	unixMode := externalAttrs >> 16
	if unixMode == 0 {
		return 0, false
	}
	mode := unixModeToGo(unixMode)

	return mode, true
}

// IsValidFormat checks if data is a valid ZIP file.
func IsValidFormat(data []byte) bool {
	if len(data) < 30 {
		return false
	}
	sig := binary.LittleEndian.Uint32(data[0:4])
	return sig == sigLocalFile
}

// goModeToUnix converts Go's os.FileMode to Unix st_mode.
func goModeToUnix(mode os.FileMode) uint32 {
	// Start with permission bits
	unixMode := uint32(mode.Perm())

	// Set file type
	switch {
	case mode&os.ModeSymlink != 0:
		unixMode |= unixModeSymlink
	case mode&os.ModeDir != 0:
		unixMode |= unixModeDir
	default:
		unixMode |= unixModeRegular
	}

	return unixMode
}

// unixModeToGo converts Unix st_mode to Go's os.FileMode.
func unixModeToGo(unixMode uint32) os.FileMode {
	// Permission bits
	mode := os.FileMode(unixMode & 0777)

	// File type
	switch unixMode & unixModeTypeMask {
	case unixModeSymlink:
		mode |= os.ModeSymlink
	case unixModeDir:
		mode |= os.ModeDir
		// Regular file has no special mode bit in Go
	}

	return mode
}
