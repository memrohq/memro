// Package filestore keeps uploaded files on local disk.
package filestore

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Local struct {
	dir string
}

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Local{dir: dir}, nil
}

var safeExt = regexp.MustCompile(`^[a-z0-9]{1,8}$`)

// Save writes data under a generated name (the client file name is never
// used as a path) and returns the stored path.
func (l *Local) Save(id uuid.UUID, originalName string, data []byte) (string, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(originalName)), ".")
	if !safeExt.MatchString(ext) {
		ext = "bin"
	}
	path := filepath.Join(l.dir, id.String()+"."+ext)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

// Remove deletes a stored file. Paths outside the storage directory and
// files that are already gone are ignored.
func (l *Local) Remove(path string) error {
	abs, err := l.inside(path)
	if err != nil {
		return nil
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *Local) inside(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(l.dir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", os.ErrNotExist
	}
	return abs, nil
}

// Read only returns files inside the storage directory.
func (l *Local) Read(path string) ([]byte, error) {
	abs, err := l.inside(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}
