package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rinodrops/rigo/internal/config"
)

// trash_meta is the per-generation metadata file naming the trashed
// vault-relative path (forget refuses vault entries with this name).
const trash_meta = ".rigo-entry"

// TrashEntry is one trash generation: a single forgotten vault entry.
type TrashEntry struct {
	Stamp    string // generation timestamp, e.g. "20260714T093000Z"
	VaultRel string // original vault-relative path (slash-separated)
	Logical  string // logical path on this host; "" when the entry belongs to another OS/distro/volume
	Target   string // deploy location on this host; "" when Logical is ""
	Content  string // absolute path of the trashed content
}

// TrashList enumerates trash generations, newest first.
func TrashList(root string, cfg *config.Config, host Host) ([]TrashEntry, error) {
	trash_root := filepath.Join(root, cfg.TrashDir)
	generations, err := os.ReadDir(trash_root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	volumes, err := resolve_volumes(cfg, host)
	if err != nil {
		return nil, err
	}

	var entries []TrashEntry
	for _, gen := range generations {
		if !gen.IsDir() {
			continue
		}
		gen_root := filepath.Join(trash_root, gen.Name())
		rel, err := generation_entry(gen_root)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", gen_root, err)
		}
		if rel == "" {
			continue // empty generation
		}
		e := TrashEntry{
			Stamp:    gen.Name(),
			VaultRel: rel,
			Content:  filepath.Join(gen_root, filepath.FromSlash(rel)),
		}
		e.Logical, e.Target = derive(cfg, host, volumes, rel)
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Stamp != entries[j].Stamp {
			return entries[i].Stamp > entries[j].Stamp
		}
		return entries[i].VaultRel < entries[j].VaultRel
	})
	return entries, nil
}

// generation_entry reads the vault-relative path a generation holds:
// from its metadata file, or (for generations without one) by
// descending single-child directories.
func generation_entry(gen_root string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(gen_root, trash_meta)); err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	rel := ""
	dir := gen_root
	for {
		items, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			if dir == gen_root && item.Name() == trash_meta {
				continue
			}
			names = append(names, item.Name())
		}
		if len(names) == 0 {
			return rel, nil // empty generation (or bare directory entry)
		}
		if len(names) > 1 {
			return rel, nil // branching: this directory is the entry
		}
		rel = filepath.ToSlash(filepath.Join(filepath.FromSlash(rel), names[0]))
		next := filepath.Join(dir, names[0])
		fi, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if !fi.IsDir() {
			return rel, nil
		}
		dir = next
	}
}

// derive maps a vault-relative path to its logical path and target on
// this host, mirroring the scanner's layer rules. It returns empty
// strings for entries that do not deploy here (another OS, an
// undeclared distro overlay, another flavour, or an unresolvable volume).
func derive(cfg *config.Config, host Host, volumes map[string]string, vault_rel string) (string, string) {
	rest, in_os := strings.CutPrefix(vault_rel, cfg.OSDir+"/")
	if !in_os {
		return vault_rel, host.target(vault_rel, false)
	}
	goos, rest, ok := strings.Cut(rest, "/")
	if !ok || goos != host.GOOS {
		return "", ""
	}
	if fl_rest, ok := strings.CutPrefix(rest, cfg.FlavourDir+"/"); ok {
		name, fl_rest, ok := strings.Cut(fl_rest, "/")
		if !ok || !KnownFlavour(name) || name != host.Flavour {
			return "", ""
		}
		rest = fl_rest
	} else if host.GOOS == "linux" {
		if first, tail, ok := strings.Cut(rest, "/"); ok && first == host.Distro && distro_listed(cfg, first) {
			rest = tail
		} else if ok && first != host.Distro && distro_listed(cfg, first) {
			return "", "" // another distro's overlay
		}
	}
	if abs_rest, ok := strings.CutPrefix(rest, cfg.AbsDir+"/"); ok {
		if host.GOOS != "windows" {
			return "/" + abs_rest, host.target(abs_rest, true)
		}
		volume, tail, ok := strings.Cut(abs_rest, "/")
		if !ok {
			return "", ""
		}
		letter, resolved := volumes[volume]
		if !resolved {
			return "", ""
		}
		return volume + ":/" + tail,
			strings.ToUpper(letter) + ":" + string(filepath.Separator) + filepath.FromSlash(tail)
	}
	if host.GOOS == "windows" {
		return rest, host.win_target(rest)
	}
	return rest, host.target(rest, false)
}

func distro_listed(cfg *config.Config, name string) bool {
	for _, d := range cfg.Distros {
		if d == name {
			return true
		}
	}
	return false
}

// Newest returns the most recent trash generation holding the logical
// path.
func Newest(entries []TrashEntry, logical string) (TrashEntry, bool) {
	want := norm(logical)
	for _, e := range entries { // already newest first
		if norm(e.Logical) == want && e.Logical != "" {
			return e, true
		}
	}
	return TrashEntry{}, false
}

// TrashRestore moves a trashed entry back to its vault location (the
// undo of forget) and removes the emptied generation directory.
func TrashRestore(root string, cfg *config.Config, e TrashEntry) error {
	dest := filepath.Join(root, filepath.FromSlash(e.VaultRel))
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("the vault already contains %s", e.VaultRel)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(e.Content, dest); err != nil {
		return err
	}
	prune_generation(filepath.Join(root, cfg.TrashDir, e.Stamp))
	return nil
}

// RestoreLocal copies a trashed entry to its target as a real file,
// replacing a symlink there. The trash copy stays: other machines may
// still need it.
func RestoreLocal(e TrashEntry) error {
	if e.Target == "" {
		return fmt.Errorf("%s does not deploy on this host", e.VaultRel)
	}
	staged := stage_path(e.Target)
	if err := copy_any(e.Content, staged); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Remove(e.Target); err != nil && !os.IsNotExist(err) {
		os.RemoveAll(staged)
		return err
	}
	return os.Rename(staged, e.Target)
}

// TrashEmpty permanently deletes generations older than cutoff (all
// when cutoff is zero) and returns the deleted stamps.
func TrashEmpty(root string, cfg *config.Config, cutoff time.Duration) ([]string, error) {
	trash_root := filepath.Join(root, cfg.TrashDir)
	generations, err := os.ReadDir(trash_root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var deleted []string
	for _, gen := range generations {
		if !gen.IsDir() {
			continue
		}
		if cutoff > 0 {
			when, err := time.Parse(trash_stamp, gen.Name())
			if err != nil || time.Since(when) < cutoff {
				continue
			}
		}
		if err := os.RemoveAll(filepath.Join(trash_root, gen.Name())); err != nil {
			return deleted, err
		}
		deleted = append(deleted, gen.Name())
	}
	sort.Strings(deleted)
	return deleted, nil
}

// prune_generation removes a generation directory that only holds
// empty parents and metadata after a restore.
func prune_generation(gen_root string) {
	os.Remove(filepath.Join(gen_root, trash_meta))
	// Remove now-empty directories bottom-up.
	var dirs []string
	filepath.WalkDir(gen_root, func(p string, d fs.DirEntry, err error) error { //nolint:errcheck
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // fails silently unless empty, which is intended
	}
}

// RemoveLink deletes a symlink, refusing anything else (clean must
// never delete real content).
func RemoveLink(target string) error {
	fi, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", target)
	}
	return os.Remove(target)
}

// Stale describes a broken symlink found by clean.
type Stale struct {
	Logical string
	Target  string     // the symlink's location
	Dest    string     // where the symlink points
	Trash   TrashEntry // newest matching trash generation
	HasCopy bool       // a trash copy exists for restoring
}

// FindStale collects broken links: scanned entries in the broken
// state, plus dangling symlinks discovered through trash entries
// (forgets propagated from other machines).
func FindStale(entries []Entry, trash []TrashEntry, root string) ([]Stale, error) {
	var stale []Stale
	seen := map[string]bool{}

	for _, e := range entries {
		state, err := Detect(e)
		if err != nil {
			continue
		}
		if state != Broken {
			continue
		}
		dest, _ := os.Readlink(e.Target)
		s := Stale{Logical: e.Path, Target: e.Target, Dest: dest}
		if t, ok := Newest(trash, e.Path); ok {
			s.Trash, s.HasCopy = t, true
		}
		stale = append(stale, s)
		seen[e.Path] = true
	}

	for _, t := range trash {
		if t.Target == "" || seen[norm(t.Logical)] || seen[t.Logical] {
			continue
		}
		fi, err := os.Lstat(t.Target)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		dest, err := os.Readlink(t.Target)
		if err != nil {
			continue
		}
		abs_dest := dest
		if !filepath.IsAbs(abs_dest) {
			abs_dest = filepath.Join(filepath.Dir(t.Target), dest)
		}
		if _, ok := under(root, filepath.Clean(abs_dest)); !ok {
			continue // points elsewhere: not rigo's link
		}
		if _, err := os.Stat(t.Target); err == nil {
			continue // resolves fine: the entry was re-added
		}
		stale = append(stale, Stale{Logical: t.Logical, Target: t.Target, Dest: dest, Trash: t, HasCopy: true})
		seen[t.Logical] = true
	}
	return stale, nil
}
