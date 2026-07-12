package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rinodrops/rigo/internal/config"
)

// norm strips the optional trailing slash from a declared path.
func norm(p string) string {
	return strings.TrimSuffix(p, "/")
}

// decls indexes the paths declared in rigo.toml (dirs + tag members).
type decls struct {
	all     map[string]bool // normalized declared paths
	slashed map[string]bool // declared with a trailing slash
	in_dirs map[string]bool // declared in the top-level dirs array
	seen    map[string]bool // found in the vault during the scan
}

func new_decls(cfg *config.Config) *decls {
	d := &decls{
		all:     map[string]bool{},
		slashed: map[string]bool{},
		in_dirs: map[string]bool{},
		seen:    map[string]bool{},
	}
	note := func(p string, from_dirs bool) {
		n := norm(p)
		d.all[n] = true
		if strings.HasSuffix(p, "/") {
			d.slashed[n] = true
		}
		if from_dirs {
			d.in_dirs[n] = true
		}
	}
	for _, p := range cfg.Dirs {
		note(p, true)
	}
	for _, paths := range cfg.Tags {
		for _, p := range paths {
			note(p, false)
		}
	}
	return d
}

// Scan enumerates the vault for this host. It returns the entries
// (sorted by logical path) and warnings for declared paths that have
// no vault counterpart.
func Scan(root string, cfg *config.Config, host Host) ([]Entry, []string, error) {
	ig, err := new_ignorer(cfg.Ignore)
	if err != nil {
		return nil, nil, err
	}
	s := &scanner{
		root:    root,
		cfg:     cfg,
		host:    host,
		ig:      ig,
		decls:   new_decls(cfg),
		entries: map[string]Entry{},
	}

	// Later layers override earlier ones on the same logical path:
	// common < OS-specific < distro-specific.
	if err := s.layer(root, false, false); err != nil {
		return nil, nil, err
	}
	os_root := filepath.Join(root, cfg.OSDir, host.GOOS)
	if err := s.layer(os_root, true, false); err != nil {
		return nil, nil, err
	}
	if err := s.layer(filepath.Join(os_root, cfg.AbsDir), true, true); err != nil {
		return nil, nil, err
	}
	if host.GOOS == "linux" && host.Distro != "" {
		distro_root := filepath.Join(os_root, host.Distro)
		if err := s.layer(distro_root, true, false); err != nil {
			return nil, nil, err
		}
		if err := s.layer(filepath.Join(distro_root, cfg.AbsDir), true, true); err != nil {
			return nil, nil, err
		}
	}

	if err := s.check_nesting(); err != nil {
		return nil, nil, err
	}
	var warnings []string
	for p := range s.decls.all {
		if !s.decls.seen[p] {
			warnings = append(warnings, fmt.Sprintf("%s is declared in rigo.toml but has no vault entry", p))
		}
	}
	sort.Strings(warnings)

	entries := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, warnings, nil
}

type scanner struct {
	root    string
	cfg     *config.Config
	host    Host
	ig      *ignorer
	decls   *decls
	entries map[string]Entry
}

// layer walks one vault layer (common, .os/<goos>, its .abs, or a
// distro overlay) rooted at dir. os_layer enables the Windows section
// mapping; abs marks a .abs subtree.
func (s *scanner) layer(dir string, os_layer, abs bool) error {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil //nolint:nilerr // an absent layer is simply empty
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		logical := filepath.ToSlash(must_rel(dir, p))
		vault_rel := filepath.ToSlash(must_rel(s.root, p))

		if s.skip_special(dir, p, logical, os_layer, abs) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if s.ig.match(vault_rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		declared := s.decls.all[logical] && !abs
		if declared {
			s.decls.seen[logical] = true
		}
		switch {
		case d.IsDir() && declared:
			s.add(logical, p, true, os_layer, abs)
			return filepath.SkipDir
		case d.IsDir():
			return nil // container: descend
		case declared && s.decls.slashed[logical]:
			return fmt.Errorf("%s is declared with a trailing slash but is a file in the vault", logical)
		case declared && s.decls.in_dirs[logical]:
			return fmt.Errorf("dirs: %s is a file in the vault; dirs may only name directories", logical)
		default:
			s.add(logical, p, false, os_layer, abs)
			return nil
		}
	})
}

// skip_special hides structural directories from the walk: the trash
// and .os dirs at the vault root, the .abs and distro dirs inside an
// OS layer (they are scanned as separate layers).
func (s *scanner) skip_special(layer_root, p, logical string, os_layer, abs bool) bool {
	if strings.Contains(logical, "/") {
		return false
	}
	if !os_layer { // vault root
		return logical == s.cfg.OSDir || logical == s.cfg.TrashDir
	}
	if abs {
		return false
	}
	if logical == s.cfg.AbsDir {
		return true
	}
	// Inside .os/linux/, a first-level directory not starting with a
	// dot is a distro overlay (dotfiles are dot-prefixed; the current
	// distro's overlay is scanned as its own layer).
	if s.host.GOOS == "linux" && filepath.Base(layer_root) == "linux" {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() && !strings.HasPrefix(logical, ".") {
			return true
		}
	}
	return false
}

func (s *scanner) add(logical, vault_path string, dir, os_layer, abs bool) {
	e := Entry{Vault: vault_path, Dir: dir}
	switch {
	case abs:
		e.Path = "/" + logical
		e.Target = s.host.target(logical, true)
	case os_layer && s.host.GOOS == "windows":
		e.Path = logical
		e.Target = s.host.win_target(logical)
	default:
		e.Path = logical
		e.Target = s.host.target(logical, false)
	}
	s.entries[e.Path] = e
}

// check_nesting rejects declarations under a directory-unit path: the
// whole directory is one symlink, so nothing inside it can be
// addressed individually.
func (s *scanner) check_nesting() error {
	for _, e := range s.entries {
		if !e.Dir {
			continue
		}
		for declared := range s.decls.all {
			if strings.HasPrefix(declared, e.Path+"/") {
				return fmt.Errorf("%s is declared inside %s, which deploys as a single directory symlink", declared, e.Path)
			}
		}
	}
	return nil
}

// must_rel is filepath.Rel for paths built from a common root, where
// failure is impossible.
func must_rel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		panic(err)
	}
	return rel
}

// Find returns the entry whose logical path matches p (trailing slash
// tolerated), or false.
func Find(entries []Entry, p string) (Entry, bool) {
	want := norm(path.Clean(p))
	for _, e := range entries {
		if e.Path == want {
			return e, true
		}
	}
	return Entry{}, false
}
