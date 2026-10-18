// Package hooks delivers webhook notifications. Delivery refuses private,
// loopback and link-local addresses so tenants cannot use webhooks to reach
// internal services such as the cloud metadata server.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

var ErrBlockedAddress = errors.New("webhook target resolves to a private or reserved address")

// ValidateURL checks a webhook URL before it is stored.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return errors.New("url must be an absolute http(s) URL")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && blocked(ip) {
		return ErrBlockedAddress
	}
	return nil
}

func blocked(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() ||
		// carrier-grade NAT 100.64.0.0/10
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64)
}

// client checks the resolved IP at connect time, which also defeats DNS
// rebinding between validation and delivery.
var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if ip := net.ParseIP(host); ip == nil || blocked(ip) {
					return ErrBlockedAddress
				}
				return nil
			},
		}).DialContext,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Notify posts payload to each URL in the background.
func Notify(urls []string, event string, payload any) {
	if len(urls) == 0 {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	go func() {
		for _, u := range urls {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := post(ctx, u, body); err != nil {
				slog.Warn("webhook delivery failed", "event", event, "url", u, "err", err)
			}
			cancel()
		}
	}()
}

func post(ctx context.Context, u string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "memro-webhooks/1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
