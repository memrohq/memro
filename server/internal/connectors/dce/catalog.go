package dce

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// The connector catalog: one YAML manifest per service, built into the
// binary. Each becomes a token connector the user connects by pasting an API
// token (ADR 0014).
//
//go:embed catalog/*.yaml
var catalogFS embed.FS

// Categories shown in the dashboard, in order.
var Categories = []string{"engineering", "knowledge", "work", "communication"}

// Catalog loads and validates every manifest in catalog/. A broken manifest
// is a build error, caught by TestCatalog, not something to skip at run time.
func Catalog() ([]*Manifest, error) {
	files, err := fs.Glob(catalogFS, "catalog/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []*Manifest
	seen := map[string]bool{}
	for _, f := range files {
		data, err := catalogFS.ReadFile(f)
		if err != nil {
			return nil, err
		}
		m, err := ParseManifestYAML(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if want := strings.TrimSuffix(path.Base(f), ".yaml"); m.Metadata.Name != want {
			return nil, fmt.Errorf("%s: metadata.name %q must match the file name", f, m.Metadata.Name)
		}
		if err := checkCatalogEntry(m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if seen[m.Metadata.Name] {
			return nil, fmt.Errorf("%s: duplicate connector %q", f, m.Metadata.Name)
		}
		seen[m.Metadata.Name] = true
		out = append(out, m)
	}
	return out, nil
}

// checkCatalogEntry holds catalog manifests to more than Validate: the
// dashboard needs a name, category and docs link, the user needs a secret
// field to paste the token into, and connecting needs a verify request.
func checkCatalogEntry(m *Manifest) error {
	md := m.Metadata
	if md.DisplayName == "" || md.DocsURL == "" {
		return fmt.Errorf("displayName and docsURL are required in the catalog")
	}
	known := false
	for _, c := range Categories {
		known = known || c == md.Category
	}
	if !known {
		return fmt.Errorf("category %q is not one of %v", md.Category, Categories)
	}
	if !strings.HasPrefix(md.DocsURL, "https://") {
		return fmt.Errorf("docsURL must be https")
	}
	if !strings.HasPrefix(m.Spec.BaseURL, "https://") {
		return fmt.Errorf("baseURL must be https")
	}
	secret := false
	for _, f := range m.Spec.Fields {
		secret = secret || f.Secret
	}
	if !secret {
		return fmt.Errorf("a catalog connector needs a secret field for the token")
	}
	if m.Spec.Verify == nil {
		return fmt.Errorf("a catalog connector needs a verify request")
	}
	return nil
}
