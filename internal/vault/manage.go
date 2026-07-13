package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rinodrops/rigo/internal/config"
)

// trash_stamp is the layout of trash generation directories (UTC).
const trash_stamp = "20060102T150405Z"

// Add moves the real content at e.Target into the vault at e.Vault and
// links it back. When the source is itself a symlink, its referent's
// content is adopted (copy semantics; the referent stays where it is)
// unless keep_symlink stores the symlink as-is. A linking failure
// rolls the content back to where it came from.
func Add(e Entry, keep_symlink bool) error {
	if err := os.MkdirAll(filepath.Dir(e.Vault), 0o755); err != nil {
		return err
	}
	fi, err := os.Lstat(e.Target)
	if err != nil {
		return err
	}

	if fi.Mode()&fs.ModeSymlink != 0 && !keep_symlink {
		real, err := filepath.EvalSymlinks(e.Target)
		if err != nil {
			return err
		}
		if err := copy_any(real, e.Vault); err != nil {
			os.RemoveAll(e.Vault)
			return err
		}
		if err := os.Remove(e.Target); err != nil {
			os.RemoveAll(e.Vault)
			return err
		}
	} else if err := move_any(e.Target, e.Vault); err != nil {
		return err
	}

	if err := Link(e); err != nil {
		if back := move_any(e.Vault, e.Target); back != nil {
			return fmt.Errorf("%w (and moving the content back failed: %v)", err, back)
		}
		return err
	}
	return nil
}

// Forget stops managing an entry: a locally deployed symlink is
// replaced with a real copy first, then the vault content moves to the
// trash. It returns the vault-relative trash location. rigo.toml
// cleanup is the caller's job.
func Forget(e Entry, root string, cfg *config.Config, deployed bool) (string, error) {
	if deployed {
		state, err := Detect(e)
		if err != nil {
			return "", err
		}
		// Only symlinks are materialized; real local content
		// (unlinked/conflict) is never touched, pending has nothing.
		if state == Linked || state == Broken {
			if err := Unlink(e); err != nil {
				return "", err
			}
		}
	}

	rel := must_rel(root, e.Vault)
	trash_rel := filepath.Join(cfg.TrashDir, time.Now().UTC().Format(trash_stamp), rel)
	dest := filepath.Join(root, trash_rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(e.Vault, dest); err != nil {
		return "", err
	}
	return filepath.ToSlash(trash_rel), nil
}

// move_any renames src to dst, falling back to copy-and-delete across
// filesystems.
func move_any(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copy_any(src, dst); err != nil {
		os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}
