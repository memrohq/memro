package email

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// signInEmail is the content of a sign-in code email.
type signInEmail struct {
	Code    string
	To      string
	LogoURL string // empty when the app isn't served over https (images wouldn't load)
	AppURL  string
}

func (e signInEmail) Subject() string { return "Your Memro sign-in code: " + e.Code }

func (e signInEmail) Text() string {
	return strings.Join([]string{
		"Your Memro sign-in code",
		"",
		"    " + e.Code,
		"",
		"Enter this code on the sign-in page. It expires in 15 minutes and works once.",
		"",
		"Didn't try to sign in? You can ignore this email. Someone may have typed your",
		"address by mistake; nobody can sign in without this code.",
		"",
		"--",
		"Memro · Long-term memory for your AI tools",
		"Sent to " + e.To + " because a sign-in was requested for this address.",
	}, "\r\n")
}

// The HTML follows what email clients support: tables for layout, inline
// styles, no web fonts or SVG. Colors hold up when Gmail and Outlook invert
// them in dark mode, and Apple Mail uses the prefers-color-scheme block.
var signInHTML = template.Must(template.New("signin").Parse(`<!DOCTYPE html>
<html lang="en" xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>Your Memro sign-in code</title>
<style>
  @media (prefers-color-scheme: dark) {
    .bg { background: #0b0c0f !important; }
    .card { background: #15161a !important; border-color: #26272d !important; }
    .ink { color: #ededf2 !important; }
    .muted { color: #a3a5b1 !important; }
    .code { background: #1d1b33 !important; border-color: #312d58 !important; color: #c7c2ff !important; }
    .rule { border-color: #26272d !important; }
  }
  @media (max-width: 600px) {
    .pad { padding: 28px 22px !important; }
    .code { font-size: 30px !important; letter-spacing: 8px !important; }
  }
</style>
</head>
<body class="bg" style="margin:0;padding:0;background:#f4f4f7;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">Your code is {{.Code}}. It expires in 15 minutes.&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;</div>
<table role="presentation" class="bg" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f4f4f7;">
  <tr><td align="center" style="padding:40px 16px;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
      <tr><td style="padding:0 4px 20px;">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
          {{if .LogoURL}}<td style="padding-right:10px;vertical-align:middle;"><img src="{{.LogoURL}}" width="28" height="28" alt="" style="display:block;border:0;"></td>{{end}}
          <td class="ink" style="vertical-align:middle;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:19px;font-weight:700;letter-spacing:-0.4px;color:#0e0f13;">memro</td>
        </tr></table>
      </td></tr>
      <tr><td class="card pad" style="background:#ffffff;border:1px solid #e6e6ec;border-radius:16px;padding:40px 40px 36px;">
        <h1 class="ink" style="margin:0 0 10px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:22px;line-height:1.3;font-weight:650;letter-spacing:-0.3px;color:#0e0f13;">Your sign-in code</h1>
        <p class="muted" style="margin:0 0 28px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#5d5f6b;">Enter this code on the Memro sign-in page. It expires in <strong class="ink" style="color:#0e0f13;">15 minutes</strong> and works once.</p>
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr>
          <td class="code" align="center" style="background:#f2f0fd;border:1px solid #dedafa;border-radius:12px;padding:22px 12px;font-family:'SF Mono',SFMono-Regular,Menlo,Consolas,'Courier New',monospace;font-size:36px;line-height:1;font-weight:700;letter-spacing:12px;color:#4535bd;">{{.Code}}</td>
        </tr></table>
        <p class="muted" style="margin:28px 0 0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:13.5px;line-height:1.6;color:#5d5f6b;"><strong class="ink" style="color:#0e0f13;">Didn't try to sign in?</strong> You can ignore this email. Someone may have typed your address by mistake; nobody can sign in without this code.</p>
      </td></tr>
      <tr><td style="padding:24px 4px 0;">
        <p class="muted" style="margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:12px;line-height:1.6;color:#8e909c;">Memro · Long-term memory for your AI tools{{if .AppURL}} · <a href="{{.AppURL}}" style="color:#8e909c;text-decoration:underline;">{{.AppURL}}</a>{{end}}<br>Sent to {{.To}} because a sign-in was requested for this address.</p>
      </td></tr>
    </table>
  </td></tr>
</table>
</body>
</html>`))

func (e signInEmail) HTML() (string, error) {
	var buf bytes.Buffer
	if err := signInHTML.Execute(&buf, e); err != nil {
		return "", fmt.Errorf("render sign-in email: %w", err)
	}
	return buf.String(), nil
}
