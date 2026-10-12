package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Mac App icons uploaded in the console are downloaded once per version and kept in
// appIconCacheDir as <app id>-<version>.png (readable by the logged-in user, who posts the
// notifications). The cache cleans itself: a new version replaces the old file, and
// pruneAppIcons drops icons of apps the server no longer lists.
var appIconCacheDir = filepath.Join(SupportDir, "cache", "app-icons")

// ErrNoIcon means the app has no uploaded icon.
var ErrNoIcon = errors.New("no uploaded icon")

var (
	iconIDRe      = regexp.MustCompile(`^[0-9a-f-]{1,64}$`)
	iconVersionRe = regexp.MustCompile(`^[0-9a-f]{12}$`)
	pngMagic      = []byte("\x89PNG\r\n\x1a\n")
)

const maxCachedIcon = 2 << 20

func cachedIconPath(id, version string) string {
	return filepath.Join(appIconCacheDir, id+"-"+version+".png")
}

// uploadedAppIcon returns the local path of a Mac App's uploaded icon. With a version (from
// the Self Service list) a cached copy is used; without one the current icon is fetched.
func uploadedAppIcon(ctx context.Context, c *Client, id, version string) (string, error) {
	if !iconIDRe.MatchString(id) || (version != "" && !iconVersionRe.MatchString(version)) {
		return "", errors.New("invalid icon reference")
	}
	if version != "" && fileExists(cachedIconPath(id, version)) {
		return cachedIconPath(id, version), nil
	}
	b, v, err := c.AppIcon(ctx, id)
	if err != nil {
		return "", err
	}
	if !iconVersionRe.MatchString(v) || len(b) > maxCachedIcon || !bytes.HasPrefix(b, pngMagic) {
		return "", errors.New("the server sent something that is not a PNG icon")
	}
	return cacheAppIcon(id, v, b)
}

func cacheAppIcon(id, version string, png []byte) (string, error) {
	p := cachedIconPath(id, version)
	if fileExists(p) {
		return p, nil
	}
	if err := os.MkdirAll(appIconCacheDir, 0o755); err != nil {
		return "", err
	}
	tmp := p + ".download"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, png, 0o644); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	old, _ := filepath.Glob(filepath.Join(appIconCacheDir, id+"-*.png"))
	for _, o := range old {
		if o != p {
			os.Remove(o)
		}
	}
	return p, nil
}

// pruneAppIcons removes cached icons whose app or version is no longer current.
// current maps app id → icon version.
func pruneAppIcons(current map[string]string) {
	ents, _ := os.ReadDir(appIconCacheDir)
	for _, e := range ents {
		base := strings.TrimSuffix(e.Name(), ".png")
		i := strings.LastIndexByte(base, '-')
		if i > 0 && strings.HasSuffix(e.Name(), ".png") && current[base[:i]] == base[i+1:] {
			continue
		}
		os.Remove(filepath.Join(appIconCacheDir, e.Name()))
	}
}
