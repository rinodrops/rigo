package vault

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
)

// Diff describes the difference between an entry's local content and
// its vault source, prepared for the conflict prompt.
type Diff struct {
	Stat    string // one-line summary: "+3 -1" or "2 added, 1 removed, 3 changed"
	Unified string // unified diff; empty for binary or directory entries
	Lines   int    // number of lines in Unified
}

// Compare builds the conflict presentation for an entry whose target
// is a real file or directory differing from the vault.
func Compare(e Entry) (Diff, error) {
	if e.Dir {
		return compare_trees(e.Vault, e.Target)
	}
	return compare_files(e.Vault, e.Target)
}

func compare_files(vault_path, target string) (Diff, error) {
	vault_bin, err := file_is_binary(vault_path)
	if err != nil {
		return Diff{}, err
	}
	local_bin, err := file_is_binary(target)
	if err != nil {
		return Diff{}, err
	}
	if vault_bin || local_bin {
		return Diff{Stat: "binary files differ"}, nil
	}

	vault_data, err := os.ReadFile(vault_path)
	if err != nil {
		return Diff{}, err
	}
	local_data, err := os.ReadFile(target)
	if err != nil {
		return Diff{}, err
	}

	unified := udiff.Unified("vault", "local", string(vault_data), string(local_data))
	added, removed := 0, 0
	for _, line := range strings.Split(unified, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return Diff{
		Stat:    fmt.Sprintf("+%d -%d", added, removed),
		Unified: unified,
		Lines:   strings.Count(unified, "\n"),
	}, nil
}

// compare_trees summarizes a directory-unit conflict as added,
// removed, and changed files (local relative to the vault).
func compare_trees(vault_path, target string) (Diff, error) {
	vault_files, err := list_tree(vault_path)
	if err != nil {
		return Diff{}, err
	}
	target_files, err := list_tree(target)
	if err == errNotDir {
		return Diff{Stat: "local is a file, vault is a directory"}, nil
	}
	if err != nil {
		return Diff{}, err
	}

	in_vault := map[string]bool{}
	for _, rel := range vault_files {
		in_vault[rel] = true
	}
	var added, removed, changed []string
	for _, rel := range target_files {
		if !in_vault[rel] {
			added = append(added, rel)
			continue
		}
		delete(in_vault, rel)
		equal, err := equal_file(filepath.Join(vault_path, rel), filepath.Join(target, rel))
		if err != nil {
			return Diff{}, err
		}
		if !equal {
			changed = append(changed, rel)
		}
	}
	for rel := range in_vault {
		removed = append(removed, rel)
	}
	sort.Strings(removed)

	var b strings.Builder
	describe := func(label string, files []string) {
		for _, rel := range files {
			fmt.Fprintf(&b, "  %s %s\n", label, rel)
		}
	}
	describe("+", added)
	describe("-", removed)
	describe("~", changed)
	return Diff{
		Stat: fmt.Sprintf("%d added, %d removed, %d changed (local vs vault)",
			len(added), len(removed), len(changed)),
		Unified: b.String(),
		Lines:   len(added) + len(removed) + len(changed),
	}, nil
}

// file_is_binary applies the git heuristic against the first 8000
// bytes of path without reading the whole file.
func file_is_binary(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}
