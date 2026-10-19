package extract

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Limits that keep a hostile file (e.g. a zip bomb) from exhausting memory.
var maxXMLPartBytes = 64 << 20 // decompressed size of one XML part (a var so tests can lower it)

const maxExtractedText = 4 << 20 // characters of text returned

var ErrUnsupported = errors.New("text cannot be extracted from this file type")

// Text extracts plain text from a file of the given type (see FileType)
// without external services: PDF, Word (.docx), PowerPoint (.pptx), Excel
// (.xlsx) and text files. Images and audio need a model; see the embed package.
func Text(fileType string, data []byte) (string, error) {
	var (
		s   string
		err error
	)
	switch fileType {
	case "pdf":
		s, err = PDFText(data)
	case "docx":
		s, err = DocxText(data)
	case "pptx":
		s, err = PptxText(data)
	case "xlsx":
		s, err = XlsxText(data)
	case "text":
		if !isUTF8(data) {
			return "", errors.New("text file is not valid UTF-8")
		}
		s = string(data)
	default:
		return "", ErrUnsupported
	}
	if err != nil {
		return "", err
	}
	s = Normalize(s)
	if r := []rune(s); len(r) > maxExtractedText {
		s = string(r[:maxExtractedText])
	}
	return s, nil
}

// DetectType refines FileType with the file's content, for uploads whose
// name has no useful extension.
func DetectType(name string, data []byte) string {
	t := FileType(name)
	if t != "unknown" {
		return t
	}
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			for _, f := range zr.File {
				switch {
				case f.Name == "word/document.xml":
					return "docx"
				case f.Name == "ppt/presentation.xml":
					return "pptx"
				case f.Name == "xl/workbook.xml":
					return "xlsx"
				}
			}
		}
	case isUTF8(data) && !bytes.ContainsRune(data, 0):
		return "text"
	}
	return "unknown"
}

func openZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a valid Office file: %w", err)
	}
	return zr, nil
}

func readPart(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, int64(maxXMLPartBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxXMLPartBytes {
		return nil, errors.New("document part is too large")
	}
	return b, nil
}

func findPart(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// walkText streams an OOXML part: text elements (local name textTag) are
// written as-is, paragraph ends become newlines, tabs and breaks are kept.
func walkText(part []byte, textTag, paraTag string, sb *strings.Builder) error {
	dec := xml.NewDecoder(bytes.NewReader(part))
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case textTag:
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case textTag:
				inText = false
			case paraTag:
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
		if sb.Len() > maxExtractedText*4 {
			return nil
		}
	}
}

// DocxText extracts the body text of a Word document, a paragraph per line.
func DocxText(data []byte) (string, error) {
	zr, err := openZip(data)
	if err != nil {
		return "", err
	}
	f := findPart(zr, "word/document.xml")
	if f == nil {
		return "", errors.New("not a Word document (word/document.xml missing)")
	}
	part, err := readPart(f)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := walkText(part, "t", "p", &sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

var reSlideName = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// PptxText extracts slide text in slide order, slides separated by a blank line.
func PptxText(data []byte) (string, error) {
	zr, err := openZip(data)
	if err != nil {
		return "", err
	}
	type slide struct {
		n int
		f *zip.File
	}
	var slides []slide
	for _, f := range zr.File {
		if m := reSlideName.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, slide{n, f})
		}
	}
	if len(slides) == 0 {
		return "", errors.New("not a PowerPoint presentation (no slides)")
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	var sb strings.Builder
	for _, s := range slides {
		part, err := readPart(s.f)
		if err != nil {
			return "", err
		}
		if err := walkText(part, "t", "p", &sb); err != nil {
			return "", err
		}
		sb.WriteString("\n\n")
	}
	return sb.String(), nil
}

var reSheetName = regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)

// XlsxText extracts each sheet's cells as tab-separated rows, sheets
// separated by a blank line.
func XlsxText(data []byte) (string, error) {
	zr, err := openZip(data)
	if err != nil {
		return "", err
	}
	var shared []string
	if f := findPart(zr, "xl/sharedStrings.xml"); f != nil {
		part, err := readPart(f)
		if err != nil {
			return "", err
		}
		shared, err = sharedStrings(part)
		if err != nil {
			return "", err
		}
	}
	type sheet struct {
		n int
		f *zip.File
	}
	var sheets []sheet
	for _, f := range zr.File {
		if m := reSheetName.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, sheet{n, f})
		}
	}
	if len(sheets) == 0 {
		return "", errors.New("not an Excel workbook (no sheets)")
	}
	sort.Slice(sheets, func(i, j int) bool { return sheets[i].n < sheets[j].n })
	var sb strings.Builder
	for _, s := range sheets {
		part, err := readPart(s.f)
		if err != nil {
			return "", err
		}
		if err := sheetText(part, shared, &sb); err != nil {
			return "", err
		}
		sb.WriteString("\n")
		if sb.Len() > maxExtractedText*4 {
			break
		}
	}
	return sb.String(), nil
}

func sharedStrings(part []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(part))
	var out []string
	var cur strings.Builder
	inSI, inT := false, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				inT = inSI
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				inSI = false
				out = append(out, cur.String())
			case "t":
				inT = false
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
}

func sheetText(part []byte, shared []string, sb *strings.Builder) error {
	dec := xml.NewDecoder(bytes.NewReader(part))
	var row []string
	var cellType string
	var val strings.Builder
	inV := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row = row[:0]
			case "c":
				cellType = ""
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						cellType = a.Value
					}
				}
				val.Reset()
			case "v", "t":
				inV = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v", "t":
				inV = false
			case "c":
				v := val.String()
				if cellType == "s" {
					if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				}
				row = append(row, v)
			case "row":
				for len(row) > 0 && row[len(row)-1] == "" {
					row = row[:len(row)-1]
				}
				if len(row) > 0 {
					sb.WriteString(strings.Join(row, "\t"))
					sb.WriteByte('\n')
				}
			}
		case xml.CharData:
			if inV {
				val.Write(t)
			}
		}
	}
}
