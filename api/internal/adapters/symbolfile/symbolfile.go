// Package symbolfile persists the tracked symbol list as a small JSON file.
package symbolfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Load reads the list. found is false when the file does not exist yet.
func Load(path string) (symbols []string, found bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(b, &symbols); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	return symbols, true, nil
}

// Save writes the list atomically (temp file + rename).
func Save(path string, symbols []string) error {
	if symbols == nil {
		symbols = []string{}
	}
	b, err := json.MarshalIndent(symbols, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
