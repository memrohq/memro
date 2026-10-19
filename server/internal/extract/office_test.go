package extract

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func TestDocxText(t *testing.T) {
	doc := zipOf(t, map[string]string{"word/document.xml": `<w:document ` + wNS + `><w:body>
		<w:p><w:r><w:t>Quarterly </w:t></w:r><w:r><w:t>planning</w:t></w:r></w:p>
		<w:p><w:r><w:t>Owner:</w:t><w:tab/><w:t>Ana &amp; Ravi</w:t></w:r></w:p>
	</w:body></w:document>`})
	got, err := Text("docx", doc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Quarterly planning\nOwner: Ana & Ravi" {
		t.Fatalf("docx text = %q", got)
	}
	if DetectType("upload", doc) != "docx" {
		t.Error("docx not detected by content")
	}
}

func TestPptxTextIsInSlideOrder(t *testing.T) {
	a := `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	slide := func(s string) string { return `<p:sld ` + a + `><a:p><a:r><a:t>` + s + `</a:t></a:r></a:p></p:sld>` }
	deck := zipOf(t, map[string]string{
		"ppt/presentation.xml":   "<p/>",
		"ppt/slides/slide10.xml": slide("Ten"),
		"ppt/slides/slide2.xml":  slide("Two"),
		"ppt/slides/slide1.xml":  slide("One"),
	})
	got, err := Text("pptx", deck)
	if err != nil {
		t.Fatal(err)
	}
	if got != "One\n\nTwo\n\nTen" {
		t.Fatalf("pptx text = %q", got)
	}
}

func TestXlsxText(t *testing.T) {
	book := zipOf(t, map[string]string{
		"xl/workbook.xml":      "<workbook/>",
		"xl/sharedStrings.xml": `<sst><si><t>Name</t></si><si><t>Team</t></si><si><r><t>A</t></r><r><t>na</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>
			<row><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row>
			<row><c t="s"><v>2</v></c><c t="inlineStr"><is><t>Platform</t></is></c><c><v>42</v></c></row>
		</sheetData></worksheet>`,
	})
	got, err := Text("xlsx", book)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Name Team\nAna Platform 42" { // Normalize turns tabs into spaces
		t.Fatalf("xlsx text = %q", got)
	}
}

func TestExtractionRejectsBadInput(t *testing.T) {
	for _, typ := range []string{"docx", "pptx", "xlsx", "pdf"} {
		if _, err := Text(typ, []byte("not a real file")); err == nil {
			t.Errorf("%s: garbage accepted", typ)
		}
	}
	if _, err := Text("text", []byte{0xff, 0xfe, 0x00}); err == nil {
		t.Error("invalid UTF-8 accepted as text")
	}
	if _, err := Text("image", nil); err != ErrUnsupported {
		t.Errorf("image: err = %v", err)
	}
	// A truncated PDF must not crash the server.
	if _, err := PDFText([]byte("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >>\nxref\n0 1\ntrailer << /Root 1 0 R /Size 99999999 >>\nstartxref\n9\n%%EOF")); err == nil {
		t.Error("broken PDF accepted")
	}
}

func TestDocxZipBombIsBounded(t *testing.T) {
	defer func(n int) { maxXMLPartBytes = n }(maxXMLPartBytes)
	maxXMLPartBytes = 1 << 16
	huge := `<w:document ` + wNS + `><w:body><w:p><w:r><w:t>` + strings.Repeat("a", maxXMLPartBytes+10) + `</w:t></w:r></w:p></w:body></w:document>`
	if _, err := DocxText(zipOf(t, map[string]string{"word/document.xml": huge})); err == nil {
		t.Error("oversized part accepted")
	}
}

func TestDetectType(t *testing.T) {
	cases := map[string][]byte{
		"pdf":     []byte("%PDF-1.7 ..."),
		"text":    []byte("plain notes"),
		"unknown": {0x00, 0x01, 0x02},
	}
	for want, data := range cases {
		if got := DetectType("noext", data); got != want {
			t.Errorf("DetectType(%q) = %s, want %s", data, got, want)
		}
	}
	if DetectType("notes.md", []byte("%PDF")) != "text" {
		t.Error("the extension should win")
	}
}
