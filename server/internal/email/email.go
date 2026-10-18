// Package email sends login codes over SMTP.
package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
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

func (s *Sender) SendOTP(to, code string) error {
	if !s.Configured() {
		return errors.New("email is not configured (set EMAIL_HOST and EMAIL_HOST_USER)")
	}
	if strings.ContainsAny(to, "\r\n") {
		return errors.New("invalid recipient")
	}
	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: Your Memro Login Code",
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		fmt.Sprintf("Your Memro OTP code is: %s\r\n\r\nThis code will expire in 15 minutes.", code),
	}, "\r\n")
	return s.send(to, []byte(msg))
}

func (s *Sender) send(to string, msg []byte) error {
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
	if err := c.Mail(s.from); err != nil {
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
