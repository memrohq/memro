package config

import "testing"

func TestPEMFromEnv(t *testing.T) {
	want := "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----"
	for _, in := range []string{want, `-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----`, "  " + want + "\n"} {
		if got := pemFromEnv(in); got != want {
			t.Fatalf("pemFromEnv(%q) = %q", in, got)
		}
	}
}

func TestConnectorCallbackDefault(t *testing.T) {
	t.Setenv("APP_URL", "https://app.example.com/")
	if got := Load().Connectors.CallbackBaseURL; got != "https://app.example.com/api" {
		t.Fatalf("callback base %q", got)
	}
}
