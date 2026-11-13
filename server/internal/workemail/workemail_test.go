package workemail

import "testing"

func TestIsWorkEmail(t *testing.T) {
	for email, want := range map[string]bool{
		"subal@vaultnel.com":   true,
		"ana@eng.vaultnel.com": true,
		"SUBAL@Memro.CO":       true,
		"me@gmail.com":         false,
		"me@GMAIL.com":         false,
		"me@outlook.com":       false,
		"me@proton.me":         false,
		"x@mailinator.com":     false, // throwaway
		"x@sub.mailinator.com": false, // subdomain of a throwaway service
		"no-at-sign":           false,
		"x@localhost":          false,
		"weird@@vaultnel.com":  true, // Domain takes the last part; sign-in validates the address
	} {
		if got := IsWorkEmail(email); got != want {
			t.Errorf("IsWorkEmail(%q) = %v, want %v", email, got, want)
		}
	}
}
