package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sitemaper/internal/model"
)

const Version = 1

type File struct {
	Version       int         `json:"version"`
	RootURL       string      `json:"root_url"`
	RootAuthority string      `json:"root_authority"`
	BuiltAt       string      `json:"built_at"`
	ExpiresAt     string      `json:"expires_at"`
	Tree          *model.Node `json:"tree"`
}

func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "sitemaper"), nil
}

func FilePath(cacheDir, rootScheme, rootAuthority string) string {
	name := sanitize(rootScheme + "_" + rootAuthority)
	return filepath.Join(cacheDir, name+".sitemaper.json")
}

func Load(path string, now time.Time) (*File, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, false, err
	}
	if f.Version != Version {
		return &f, false, fmt.Errorf("unsupported cache version %d", f.Version)
	}
	exp, err := time.Parse(time.RFC3339, f.ExpiresAt)
	if err != nil {
		return &f, false, err
	}
	if now.Before(exp) || now.Equal(exp) {
		return &f, true, nil
	}
	return &f, false, nil
}

func Save(path, rootURL, rootAuthority string, ttl time.Duration, tree *model.Node, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := File{
		Version:       Version,
		RootURL:       rootURL,
		RootAuthority: rootAuthority,
		BuiltAt:       now.UTC().Format(time.RFC3339),
		ExpiresAt:     now.Add(ttl).UTC().Format(time.RFC3339),
		Tree:          tree,
	}
	b, err := json.MarshalIndent(&f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
