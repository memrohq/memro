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

// Read only returns files inside the storage directory.
func (l *Local) Read(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(l.dir)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(abs)
}
