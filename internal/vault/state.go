package vault

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// State is the deployment state of an entry on this machine.
type State int

const (
	Linked   State = iota // target is a symlink to the vault source
	Pending               // target does not exist
	Unlinked              // target is real and identical to the vault
	Conflict              // target is real and differs from the vault
	Broken                // target is a symlink, but not to the vault source
)

func (s State) String() string {
	switch s {
	case Linked:
		return "linked"
	case Pending:
		return "pending"
	case Unlinked:
		return "unlinked"
	case Conflict:
		return "conflict"
	case Broken:
		return "broken"
	}
	return "unknown"
}

// Detect determines the entry's state by inspecting the target path.
func Detect(e Entry) (State, error) {
	fi, err := os.Lstat(e.Target)
	if errors.Is(err, fs.ErrNotExist) {
		return Pending, nil
	}
	if err != nil {
		return 0, err
	}

	if fi.Mode()&fs.ModeSymlink != 0 {
		dest, err := filepath.EvalSymlinks(e.Target)
		if err != nil {
			return Broken, nil // dangling link
		}
		source, err := filepath.EvalSymlinks(e.Vault)
		if err != nil {
			return 0, err
		}
		if dest == source {
			return Linked, nil
		}
		return Broken, nil // links elsewhere
	}

	equal, err := equal_content(e.Vault, e.Target, e.Dir)
	if err != nil {
		return 0, err
	}
	if equal {
		return Unlinked, nil
	}
	return Conflict, nil
}

// equal_content compares the vault source with a real target; dir
// entries compare recursively (same file set, same contents).
func equal_content(vault_path, target string, dir bool) (bool, error) {
	if !dir {
		return equal_file(vault_path, target)
	}

	vault_files, err := list_tree(vault_path)
	if err != nil {
		return false, err
	}
	target_files, err := list_tree(target)
	if err != nil {
		// A file where a directory is expected simply differs.
		if errors.Is(err, errNotDir) {
			return false, nil
		}
		return false, err
	}
	if len(vault_files) != len(target_files) {
		return false, nil
	}
	for i, rel := range vault_files {
		if rel != target_files[i] {
			return false, nil
		}
		equal, err := equal_file(filepath.Join(vault_path, rel), filepath.Join(target, rel))
		if err != nil || !equal {
			return equal, err
		}
	}
	return true, nil
}

var errNotDir = errors.New("not a directory")

// list_tree returns the sorted, slash-separated relative paths of all
// files under root.
func list_tree(root string) ([]string, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errNotDir
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, filepath.ToSlash(must_rel(root, p)))
		}
		return nil
	})
	return files, err
}

func equal_file(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if fa.IsDir() != fb.IsDir() {
		return false, nil
	}
	if fa.Size() != fb.Size() {
		return false, nil
	}
	ra, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer ra.Close()
	rb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer rb.Close()

	bufa := make([]byte, 32*1024)
	bufb := make([]byte, 32*1024)
	for {
		na, ea := ra.Read(bufa)
		nb, eb := rb.Read(bufb)
		if na > 0 || nb > 0 {
			if na != nb || !bytes.Equal(bufa[:na], bufb[:nb]) {
				return false, nil
			}
		}
		if ea == io.EOF && eb == io.EOF {
			return true, nil
		}
		if ea != nil && ea != io.EOF {
			return false, ea
		}
		if eb != nil && eb != io.EOF {
			return false, eb
		}
		if ea == io.EOF || eb == io.EOF {
			return false, nil // one ended early despite equal sizes
		}
	}
}
