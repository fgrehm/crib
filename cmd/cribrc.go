package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// dotfilesRC holds per-project dotfiles settings from .cribrc.
type dotfilesRC struct {
	Disabled       bool
	Repository     string
	TargetPath     string
	InstallCommand string
}

// cribRC holds values loaded from a .cribrc file.
type cribRC struct {
	Config   string   // devcontainer config directory (same as --config / -C)
	Cache    []string // package cache providers (e.g. "npm", "pip", "go")
	Dotfiles dotfilesRC
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
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "config":
			rc.Config = val
		case "cache":
			for p := range strings.SplitSeq(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					rc.Cache = append(rc.Cache, p)
				}
			}
		case "dotfiles":
			if val == "false" {
				rc.Dotfiles.Disabled = true
			}
		case "dotfiles.repository":
			rc.Dotfiles.Repository = val
		case "dotfiles.targetPath":
			rc.Dotfiles.TargetPath = val
		case "dotfiles.installCommand":
			rc.Dotfiles.InstallCommand = val
		}
	}
	return rc, scanner.Err()
}
