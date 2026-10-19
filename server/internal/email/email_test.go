package email

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// parse reads a built message the way a mail client would.
func parse(t *testing.T, raw []byte) (*mail.Message, map[string]string) {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type %q: %v", m.Header.Get("Content-Type"), err)
	}
	parts := map[string]string{}
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		var r io.Reader = p
		if p.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
			r = quotedprintable.NewReader(p)
		}
		b, _ := io.ReadAll(r)
		parts[ct] = string(b)
	}
	return m, parts
}

func TestSignInEmail(t *testing.T) {
	s := New("smtp.example.com", 587, "u", "p", "login@memro.co")
	raw, envelope, err := s.signInMessage("ana@example.com", "017782", "https://app.memro.co/", time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if envelope != "login@memro.co" {
		t.Errorf("envelope sender = %q", envelope)
	}
	m, parts := parse(t, raw)
	if got := m.Header.Get("From"); got != `"Memro" <login@memro.co>` {
		t.Errorf("From = %q", got)
	}
	subj, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if subj != "Your Memro sign-in code: 017782" {
		t.Errorf("Subject = %q", subj)
	}
	if m.Header.Get("Message-ID") == "" || !strings.HasSuffix(m.Header.Get("Message-ID"), "@memro.co>") {
		t.Errorf("Message-ID = %q", m.Header.Get("Message-ID"))
	}
	text, html := parts["text/plain"], parts["text/html"]
	for name, body := range map[string]string{"text": text, "html": html} {
		for _, want := range []string{"017782", "15 minutes", "ana@example.com"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s part is missing %q", name, want)
			}
		}
	}
	for _, want := range []string{`https://app.memro.co/memro-mark.png`, `name="color-scheme"`, "prefers-color-scheme: dark", "Didn't try to sign in?"} {
		if !strings.Contains(html, want) {
			t.Errorf("html is missing %q", want)
		}
	}
	if strings.Contains(html, "<svg") || strings.Contains(html, "<script") {
		t.Error("html contains svg or script")
	}
}

func TestSignInEmailKeepsAnExistingDisplayName(t *testing.T) {
	s := New("smtp.example.com", 587, "", "", "Memro Team <hello@memro.co>")
	raw, envelope, err := s.signInMessage("ana@example.com", "123456", "http://localhost:5173", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m, parts := parse(t, raw)
	if m.Header.Get("From") != `"Memro Team" <hello@memro.co>` || envelope != "hello@memro.co" {
		t.Errorf("From = %q envelope = %q", m.Header.Get("From"), envelope)
	}
	// Images from a non-https app URL wouldn't load in mail clients; leave them out.
	if strings.Contains(parts["text/html"], "<img") || strings.Contains(parts["text/html"], "localhost") {
		t.Error("non-https app URL used in the email")
	}
}

func TestSignInEmailRejectsHeaderInjection(t *testing.T) {
	s := New("smtp.example.com", 587, "", "", "login@memro.co")
	for _, to := range []string{"ana@example.com\r\nBcc: x@evil.test", "not an address"} {
		if _, _, err := s.signInMessage(to, "123456", "", time.Now()); err == nil {
			t.Errorf("accepted recipient %q", to)
		}
	}
}

func TestSignInEmailEscapesHTML(t *testing.T) {
	html, err := signInEmail{Code: "<b>1</b>", To: `"><script>x</script>`}.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>x") || strings.Contains(html, "<b>1</b>") {
		t.Fatal("template did not escape its input")
	}
}
