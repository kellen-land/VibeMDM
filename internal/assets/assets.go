// Package assets prepares the assets directory (VIBEMDM_ASSETS_DIR): the files admins upload or
// VibeMDM builds and Macs download, kept apart from the rest of the data directory so it can be
// bind-mounted to its own host folder (and backed up or browsed there).
//
//	packages/        uploaded installer packages
//	agent-package/   the signed agent package (auto-install at enrollment)
//	app-icons/       Mac App icons (<app id>-<version>.png)
package assets

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// Subdirectories of the assets directory.
const (
	Packages     = "packages"
	AgentPackage = "agent-package"
	AppIcons     = "app-icons"
)

var subdirs = []string{Packages, AgentPackage, AppIcons}

// Prepare creates the assets directory layout and checks it is writable. When the assets
// directory is not the data directory, files an older version kept in the data directory are
// moved over (once: a file already present in the assets directory is left alone).
func Prepare(dataDir, assetsDir string, log *slog.Logger) error {
	for _, d := range subdirs {
		if err := os.MkdirAll(filepath.Join(assetsDir, d), 0o755); err != nil {
			return writableErr(assetsDir, err)
		}
	}
	probe := filepath.Join(assetsDir, ".write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return writableErr(assetsDir, err)
	}
	os.Remove(probe)
	if same(dataDir, assetsDir) {
		return nil
	}
	for _, d := range []string{Packages, AgentPackage} {
		n, err := moveFiles(filepath.Join(dataDir, d), filepath.Join(assetsDir, d))
		if n > 0 {
			log.Info("moved files to the assets directory", "folder", d, "files", n, "to", assetsDir)
		}
		if err != nil {
			return fmt.Errorf("moving %s to the assets directory: %w", d, err)
		}
	}
	return nil
}

func writableErr(dir string, err error) error {
	return fmt.Errorf("the assets directory %s is not writable by this process (uid %d): %w; "+
		"with a bind mount, give the host folder to that user (chown) or set the container's user", dir, os.Getuid(), err)
}

func same(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	if aa == bb {
		return true
	}
	ai, err1 := os.Stat(aa)
	bi, err2 := os.Stat(bb)
	return err1 == nil && err2 == nil && os.SameFile(ai, bi)
}

// moveFiles moves the regular files of src into dst (copying when they are on different
// filesystems, as two Docker mounts are) and removes src once it is empty.
func moveFiles(src, dst string) (int, error) {
	ents, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		if !e.Type().IsRegular() {
			continue
		}
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			if err := copyFile(from, to); err != nil {
				return n, err
			}
			os.Remove(from)
		}
		n++
	}
	os.Remove(src) // only succeeds when empty
	return n, nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	tmp := to + ".moving"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}
