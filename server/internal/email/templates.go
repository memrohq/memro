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
		"Sign in to Memro",
		"",
		"Enter this code on the sign-in page to continue as " + e.To + ":",
		"",
		"    " + e.Code,
		"",
		"It expires in 15 minutes and works once.",
		"",
		"Didn't try to sign in? You can ignore this email. Someone may have typed your",
		"address by mistake; nobody can sign in without this code. Memro will never",
		"ask you to share it.",
		"",
		"--",
		"Memro · Verified memory for AI agents",
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
    .card { background: #131418 !important; border-color: #24252b !important; }
    .ink { color: #ededf2 !important; }
    .muted { color: #a3a5b1 !important; }
    .faint { color: #75778a !important; }
    .codebox { background: #0e0f13 !important; border-color: #2a2b33 !important; }
    .code { color: #f4f4f8 !important; }
    .rule { border-color: #24252b !important; }
  }
  @media (max-width: 600px) {
    .pad { padding: 32px 22px 28px !important; }
    .code { font-size: 32px !important; letter-spacing: 8px !important; }
  }
</style>
</head>
<body class="bg" style="margin:0;padding:0;background:#f4f4f7;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">Your code is {{.Code}}. It expires in 15 minutes and works once.&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;&#8204;&nbsp;</div>
<table role="presentation" class="bg" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f4f4f7;">
  <tr><td align="center" style="padding:40px 16px 48px;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;">
      <tr><td style="padding:0 2px 22px;">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
          {{if .LogoURL}}<td style="padding-right:10px;vertical-align:middle;"><img src="{{.LogoURL}}" width="26" height="26" alt="" style="display:block;border:0;"></td>{{end}}
          <td class="ink" style="vertical-align:middle;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:18px;font-weight:700;letter-spacing:-0.4px;color:#0e0f13;">memro</td>
        </tr></table>
      </td></tr>
      <tr><td class="card pad" style="background:#ffffff;border:1px solid #e6e6ec;border-radius:14px;padding:40px 40px 32px;">
        <h1 class="ink" style="margin:0 0 8px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:22px;line-height:1.3;font-weight:650;letter-spacing:-0.4px;color:#0e0f13;">Sign in to Memro</h1>
        <p class="muted" style="margin:0 0 28px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#5d5f6b;">Enter this code on the sign-in page to continue as <strong class="ink" style="font-weight:600;color:#0e0f13;">{{.To}}</strong>.</p>
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
          <tr><td height="3" style="height:3px;line-height:3px;font-size:0;border-radius:10px 10px 0 0;background-color:#6a5ae0;background-image:linear-gradient(90deg,#5546d6 0%,#7a9bf0 55%,#6fb3a6 100%);">&nbsp;</td></tr>
          <tr><td class="codebox" align="center" style="background:#fafafb;border:1px solid #e6e6ec;border-top:0;border-radius:0 0 10px 10px;padding:26px 12px 22px;">
            <div class="faint" style="font-family:'SF Mono',SFMono-Regular,Menlo,Consolas,'Courier New',monospace;font-size:11px;line-height:1;letter-spacing:1.6px;text-transform:uppercase;color:#8e909c;margin:0 0 14px;">Sign-in code</div>
            <div class="code" style="font-family:'SF Mono',SFMono-Regular,Menlo,Consolas,'Courier New',monospace;font-size:38px;line-height:1;font-weight:700;letter-spacing:12px;color:#0e0f13;padding-left:12px;">{{.Code}}</div>
          </td></tr>
        </table>
        <p class="faint" style="margin:14px 0 0;text-align:center;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:13px;line-height:1.5;color:#8e909c;">Expires in 15 minutes &middot; works once</p>
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-top:28px;"><tr><td class="rule" style="border-top:1px solid #ececf1;font-size:0;line-height:0;">&nbsp;</td></tr></table>
        <p class="muted" style="margin:20px 0 0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:13.5px;line-height:1.6;color:#5d5f6b;"><strong class="ink" style="font-weight:600;color:#0e0f13;">Didn't try to sign in?</strong> You can ignore this email. Someone may have typed your address by mistake; nobody can sign in without this code. Memro will never ask you to share it.</p>
      </td></tr>
      <tr><td style="padding:22px 2px 0;">
        <p class="faint" style="margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;font-size:12px;line-height:1.7;color:#8e909c;">Memro &middot; Verified memory for AI agents{{if .AppURL}} &middot; <a href="{{.AppURL}}" style="color:#8e909c;text-decoration:underline;">Open Memro</a>{{end}}<br>Sent to {{.To}} because a sign-in was requested for this address.</p>
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
