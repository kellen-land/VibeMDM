package appicons

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Dir keeps the icons as files in the assets directory: <app id>-<version>.png, where the
// version is the start of the PNG's SHA-256. The name alone says which icon is current, so
// listing the folder is enough to build Self Service's cache keys.
type Dir struct{ Path string }

// ErrNotFound means the app has no icon.
var ErrNotFound = errors.New("no icon")

func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Versions maps app id → icon version for every stored icon.
func (d Dir) Versions() map[string]string {
	out := map[string]string{}
	ents, _ := os.ReadDir(d.Path)
	for _, e := range ents {
		if id, v, ok := parseName(e.Name()); ok {
			out[id] = v
		}
	}
	return out
}

func parseName(n string) (id, version string, ok bool) {
	base, found := strings.CutSuffix(n, ".png")
	i := strings.LastIndexByte(base, '-')
	if !found || i <= 0 || !validID(base[:i]) || len(base)-i-1 != 12 {
		return "", "", false
	}
	return base[:i], base[i+1:], true
}

// Version returns the app's icon version ("" when it has none).
func (d Dir) Version(appID string) string {
	if !validID(appID) {
		return ""
	}
	m, _ := filepath.Glob(filepath.Join(d.Path, appID+"-*.png"))
	for _, p := range m {
		if id, v, ok := parseName(filepath.Base(p)); ok && id == appID {
			return v
		}
	}
	return ""
}

// Read returns the app's icon PNG and its version.
func (d Dir) Read(appID string) ([]byte, string, error) {
	v := d.Version(appID)
	if v == "" {
		return nil, "", ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(d.Path, appID+"-"+v+".png"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	return b, v, err
}

// Save stores a sanitized PNG as the app's icon, replacing any previous one.
func (d Dir) Save(appID string, png []byte) (string, error) {
	if !validID(appID) {
		return "", errors.New("invalid app id")
	}
	if err := os.MkdirAll(d.Path, 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256(png)
	v := hex.EncodeToString(sum[:])[:12]
	tmp, err := os.CreateTemp(d.Path, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(png); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	os.Chmod(tmp.Name(), 0o644)
	if err := os.Rename(tmp.Name(), filepath.Join(d.Path, appID+"-"+v+".png")); err != nil {
		return "", err
	}
	d.removeExcept(appID, v)
	return v, nil
}

// Delete removes the app's icon (no error when it has none).
func (d Dir) Delete(appID string) error {
	if !validID(appID) {
		return errors.New("invalid app id")
	}
	d.removeExcept(appID, "")
	return nil
}

func (d Dir) removeExcept(appID, keep string) {
	m, _ := filepath.Glob(filepath.Join(d.Path, appID+"-*.png"))
	for _, p := range m {
		if id, v, ok := parseName(filepath.Base(p)); ok && id == appID && v != keep {
			os.Remove(p)
		}
	}
}

// Prune removes icons of apps that no longer exist and leftover partial uploads.
func (d Dir) Prune(exists func(appID string) bool) int {
	n := 0
	ents, _ := os.ReadDir(d.Path)
	for _, e := range ents {
		id, _, ok := parseName(e.Name())
		if (ok && !exists(id)) || strings.HasPrefix(e.Name(), ".upload-") {
			if os.Remove(filepath.Join(d.Path, e.Name())) == nil {
				n++
			}
		}
	}
	return n
}
