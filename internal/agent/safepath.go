package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// rootOwnedExecutable resolves path (following symlinks) and returns the real file only if it
// and every directory above it are owned by root and not writable by group or others. The agent
// runs as root, so executing a helper that a local user could replace would hand them root.
// Callers execute the returned path, not the symlink, so the check and the exec see one file.
func rootOwnedExecutable(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return "", err
	}
	for p := real; ; p = filepath.Dir(p) {
		fi, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if err := rootOnly(p, fi); err != nil {
			return "", err
		}
		if p == filepath.Dir(p) {
			return real, nil
		}
	}
}

func rootOnly(p string, fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: cannot read owner", p)
	}
	if st.Uid != 0 {
		return fmt.Errorf("%s is not owned by root", p)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by non-root users", p)
	}
	return nil
}
