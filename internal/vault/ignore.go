package vault

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Names and basename globs that are never managed: OS droppings and
// sync-tool artifacts that would otherwise become vault entries.
var builtin_ignore_names = map[string]bool{
	".DS_Store":   true,
	".localized":  true,
	"Thumbs.db":   true,
	"desktop.ini": true,
	".stfolder":   true,
	".stversions": true,
	".stignore":   true,
}

var builtin_ignore_globs = []string{
	".syncthing.*.tmp",
	"*.sync-conflict-*",
}

// ignorer matches vault-relative paths against the built-in ignores
// and the user patterns from rigo.toml (gitignore-style: a pattern
// without a slash matches basenames at any depth, ** crosses
// directories, a trailing slash restricts to directories).
type ignorer struct {
	patterns []ignore_pattern
}

type ignore_pattern struct {
	glob     string
	base     bool // match against the basename, not the full path
	dir_only bool
}

func new_ignorer(patterns []string) (*ignorer, error) {
	ig := &ignorer{}
	for _, p := range patterns {
		pat := ignore_pattern{glob: p}
		if strings.HasSuffix(pat.glob, "/") {
			pat.dir_only = true
			pat.glob = strings.TrimSuffix(pat.glob, "/")
		}
		pat.glob = strings.TrimPrefix(pat.glob, "/")
		pat.base = !strings.Contains(pat.glob, "/")
		if !doublestar.ValidatePattern(pat.glob) {
			return nil, fmt.Errorf("ignore: invalid pattern %q", p)
		}
		ig.patterns = append(ig.patterns, pat)
	}
	return ig, nil
}

// match reports whether the vault-relative path rel (slash-separated)
// is ignored. Matched directories are skipped whole by the scanner.
func (ig *ignorer) match(rel string, is_dir bool) bool {
	base := rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		base = rel[i+1:]
	}
	if builtin_ignore_names[base] {
		return true
	}
	for _, g := range builtin_ignore_globs {
		if ok, _ := doublestar.Match(g, base); ok {
			return true
		}
	}
	for _, p := range ig.patterns {
		if p.dir_only && !is_dir {
			continue
		}
		subject := rel
		if p.base {
			subject = base
		}
		if ok, _ := doublestar.Match(p.glob, subject); ok {
			return true
		}
	}
	return false
}
