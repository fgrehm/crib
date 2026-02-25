package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// cribRC holds values loaded from a .cribrc file.
type cribRC struct {
	Config string // devcontainer config directory (same as --config / -C)
}

// loadCribRC reads a .cribrc file from cwd. Returns nil, nil if not found.
// Format: simple "key = value" pairs, lines starting with # are comments.
func loadCribRC() (*cribRC, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(cwd, ".cribrc"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rc := &cribRC{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "config":
			rc.Config = strings.TrimSpace(val)
		}
	}
	return rc, scanner.Err()
}
