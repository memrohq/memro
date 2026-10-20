package dce

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry manages compiled and validated connector manifests.
type Registry struct {
	mu        sync.RWMutex
	manifests map[string]*Manifest
}

// NewRegistry initializes an empty manifest registry.
func NewRegistry() *Registry {
	return &Registry{
		manifests: make(map[string]*Manifest),
	}
}

// Register validates and registers a manifest.
func (r *Registry) Register(m *Manifest) error {
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validate manifest %s: %w", m.Metadata.Name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := strings.ToLower(m.Metadata.Name)
	r.manifests[name] = m
	return nil
}

// Get returns a registered manifest by lowercase name.
func (r *Registry) Get(name string) (*Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.manifests[strings.ToLower(name)]
	return m, ok
}

// All returns a slice of all registered manifests.
func (r *Registry) All() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Manifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		list = append(list, m)
	}
	return list
}

// Count returns the number of registered manifests.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.manifests)
}

// LoadFromDir reads all YAML/JSON manifests from a directory on disk.
func (r *Registry) LoadFromDir(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // catalog dir optional
		}
		return fmt.Errorf("read catalog directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("read file %s: %w", entry.Name(), err)
		}

		var m *Manifest
		if ext == ".json" {
			m, err = ParseManifestJSON(data)
		} else {
			m, err = ParseManifestYAML(data)
		}
		if err != nil {
			return fmt.Errorf("parse manifest %s: %w", entry.Name(), err)
		}

		if err := r.Register(m); err != nil {
			return err
		}
	}

	return nil
}

// LoadFromFS reads all YAML/JSON manifests from an fs.FS filesystem (e.g. embed.FS).
func (r *Registry) LoadFromFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read fs dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return fmt.Errorf("read file %s: %w", entry.Name(), err)
		}

		var m *Manifest
		if ext == ".json" {
			m, err = ParseManifestJSON(data)
		} else {
			m, err = ParseManifestYAML(data)
		}
		if err != nil {
			return fmt.Errorf("parse manifest %s: %w", entry.Name(), err)
		}

		if err := r.Register(m); err != nil {
			return err
		}
	}

	return nil
}
