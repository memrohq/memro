// Package workemail tells company addresses from free and throwaway ones
// (ADR 0027). It only decides what the dashboard offers; it never grants
// rights: a domain is proven with DNS (see the api package).
//
// The lists are embedded so nothing is looked up at request time. Refresh them with:
//
//	curl -fsSL https://raw.githubusercontent.com/Kikobeats/free-email-domains/master/domains.json \
//	  | python3 -c "import json,sys;print('\n'.join(sorted({d.strip().lower() for d in json.load(sys.stdin) if d.strip()})))" > free.txt
//	curl -fsSL https://raw.githubusercontent.com/disposable-email-domains/disposable-email-domains/main/disposable_email_blocklist.conf \
//	  | tr 'A-Z' 'a-z' | grep -v '^#' | sed '/^$/d' | sort -u > disposable.txt
package workemail

import (
	_ "embed"
	"strings"
)

var (
	//go:embed free.txt
	freeList string
	//go:embed disposable.txt
	disposableList string

	free       = toSet(freeList)
	disposable = toSet(disposableList)
)

func toSet(list string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, d := range strings.Fields(list) {
		m[d] = struct{}{}
	}
	return m
}

// Domain returns the lower-cased part after the last "@", or "".
func Domain(email string) string {
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(email[i+1:])), ".")
}

// IsWorkDomain reports whether a domain may belong to a company: not a free
// mailbox provider, not a throwaway service (or a subdomain of either).
func IsWorkDomain(domain string) bool {
	if !strings.Contains(domain, ".") {
		return false
	}
	for d := domain; strings.Contains(d, "."); d = d[strings.IndexByte(d, '.')+1:] {
		if _, ok := free[d]; ok {
			return false
		}
		if _, ok := disposable[d]; ok {
			return false
		}
	}
	return true
}

// IsWorkEmail reports whether an address is at a work domain.
func IsWorkEmail(email string) bool { return IsWorkDomain(Domain(email)) }
