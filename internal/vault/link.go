package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// Link replaces the target with a symlink to the entry's vault source.
// Existing content is renamed aside first and restored if the symlink
// cannot be created, so a failure never loses the local version.
func Link(e Entry) error {
	if err := os.MkdirAll(filepath.Dir(e.Target), 0o755); err != nil {
		return err
	}

	staged := ""
	if _, err := os.Lstat(e.Target); err == nil {
		staged = stage_path(e.Target)
		if err := os.Rename(e.Target, staged); err != nil {
			return err
		}
	}
	if err := os.Symlink(e.Vault, e.Target); err != nil {
		if staged != "" {
			if restore := os.Rename(staged, e.Target); restore != nil {
				return fmt.Errorf("%w (and restoring the original failed: %v)", err, restore)
			}
		}
		return symlink_err(err)
	}
	if staged != "" {
		return os.RemoveAll(staged)
	}
	return nil
}

// Unlink materializes a linked entry: the symlink is replaced by a
// real copy of the vault content. The copy is prepared next to the
// target and swapped in, so a failure leaves the symlink untouched.
func Unlink(e Entry) error {
	staged := stage_path(e.Target)
	if err := copy_any(e.Vault, staged); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Remove(e.Target); err != nil {
		os.RemoveAll(staged)
		return err
	}
	return os.Rename(staged, e.Target)
}

// Adopt takes the local (target) content into the vault, replacing the
// vault version, and then links the entry. This is conflict choice 2.
func Adopt(e Entry) error {
	staged := stage_path(e.Vault)
	if err := copy_any(e.Target, staged); err != nil {
		os.RemoveAll(staged)
		return err
	}
	old := stage_path(e.Vault + ".old")
	if err := os.Rename(e.Vault, old); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, e.Vault); err != nil {
		os.Rename(old, e.Vault) //nolint:errcheck // best-effort rollback
		os.RemoveAll(staged)
		return err
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	return Link(e)
}

// stage_path names a sibling temp path used while swapping content;
// staying on the same filesystem keeps renames atomic.
func stage_path(p string) string {
	return filepath.Join(filepath.Dir(p), fmt.Sprintf(".%s.rigo-%d~", filepath.Base(p), os.Getpid()))
}

// symlink_err decorates symlink failures on Windows, where creation
// needs Developer Mode or elevation.
func symlink_err(err error) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%w (creating symlinks on Windows requires Developer Mode or an elevated prompt)", err)
	}
	return err
}

// copy_any copies a file, directory tree, or symlink. Modes are
// preserved; symlinks inside a tree are recreated, not followed.
func copy_any(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		dest, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(dest, dst)
	case fi.IsDir():
		if err := os.MkdirAll(dst, fi.Mode().Perm()); err != nil {
			return err
		}
		items, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := copy_any(filepath.Join(src, item.Name()), filepath.Join(dst, item.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, fi.Mode().Perm())
	}
}
