package store

import "testing"

func TestNormalizeResource(t *testing.T) {
	for in, want := range map[string]string{
		"  billing ":              "billing",
		"./src//auth.go":          "src/auth.go",
		"src/a/../b.go":           "src/b.go",
		"https://x.example//a//b": "https://x.example//a//b",
		"Billing Service":         "Billing Service",
		"":                        "",
	} {
		if got := NormalizeResource(in); got != want {
			t.Errorf("NormalizeResource(%q) = %q, want %q", in, got, want)
		}
	}
}
