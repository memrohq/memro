// Package email sends Memro's emails (today: sign-in codes) over SMTP, as
// multipart plain text + HTML.
package email

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type Sender struct {
	host, user, pass, from string
	port                   int
}

func New(host string, port int, user, pass, from string) *Sender {
	return &Sender{host: host, port: port, user: user, pass: pass, from: from}
}

func (s *Sender) Configured() bool { return s.host != "" && s.from != "" }

// fromAddress returns the From header (with the display name "Memro" unless
// EMAIL_FROM already has one) and the bare address for the SMTP envelope.
func (s *Sender) fromAddress() (header, envelope string, err error) {
	a, err := mail.ParseAddress(s.from)
	if err != nil {
		return "", "", fmt.Errorf("invalid EMAIL_FROM %q: %w", s.from, err)
	}
	if a.Name == "" {
		a.Name = "Memro"
	}
	return a.String(), a.Address, nil
}

// SendOTP emails a sign-in code. appURL is the dashboard's URL, used for the
// logo and footer link when it is served over https.
func (s *Sender) SendOTP(to, code, appURL string) error {
	if !s.Configured() {
		return errors.New("email is not configured (set EMAIL_HOST and EMAIL_HOST_USER)")
	}
	msg, envelope, err := s.signInMessage(to, code, appURL, time.Now())
	if err != nil {
		return err
	}
	return s.send(envelope, to, msg)
}

func (s *Sender) signInMessage(to, code, appURL string, now time.Time) (msg []byte, envelope string, err error) {
	if strings.ContainsAny(to, "\r\n") {
		return nil, "", errors.New("invalid recipient")
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return nil, "", errors.New("invalid recipient")
	}
	e := signInEmail{Code: code, To: to}
	if strings.HasPrefix(appURL, "https://") {
		e.AppURL = strings.TrimRight(appURL, "/")
		e.LogoURL = e.AppURL + "/memro-mark.png"
	}
	html, err := e.HTML()
	if err != nil {
		return nil, "", err
	}
	from, envelope, err := s.fromAddress()
	if err != nil {
		return nil, "", err
	}
	msg, err = buildMessage(from, envelope, to, e.Subject(), e.Text(), html, now)
	return msg, envelope, err
}

// buildMessage assembles a multipart/alternative message (plain text first,
// then HTML), each part quoted-printable.
func buildMessage(from, envelope, to, subject, text, html string, now time.Time) ([]byte, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ ctype, content string }{
		{"text/plain; charset=UTF-8", text},
		{"text/html; charset=UTF-8", html},
	} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.ctype},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(pw)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	domain := "memro.local"
	if _, d, ok := strings.Cut(envelope, "@"); ok {
		domain = d
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	var msg bytes.Buffer
	for _, h := range [][2]string{
		{"From", from},
		{"To", to},
		{"Subject", mime.QEncoding.Encode("UTF-8", subject)},
		{"Date", now.UTC().Format(time.RFC1123Z)},
		{"Message-ID", "<" + hex.EncodeToString(id) + "@" + domain + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "multipart/alternative; boundary=" + mw.Boundary()},
	} {
		msg.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	msg.WriteString("\r\n")
	msg.Write(body.Bytes())
	return msg.Bytes(), nil
}

func (s *Sender) send(envelopeFrom, to string, msg []byte) error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}

	var c *smtp.Client
	if s.port == 465 { // implicit TLS
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, s.host); err != nil {
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return err
		}
		if c, err = smtp.NewClient(conn, s.host); err != nil {
			return err
		}
		if ok, _ := c.Extension("STARTTLS"); !ok {
			c.Close()
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			c.Close()
			return err
		}
	}
	defer c.Close()
	if s.user != "" {
		if err := c.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(envelopeFrom); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
