package vault

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rinodrops/rigo/internal/config"
)

// Route describes where a real path belongs in the vault.
type Route struct {
	Logical  string // entry logical path (volume-qualified for windows abs)
	VaultRel string // vault-relative destination, slash-separated
	// Windows absolute paths outside home need a volume decision:
	NeedsVolume bool
	Drive       string // lowercase drive letter of the source path
	Suggest     string // volume name to offer as the interactive default
	rest        string // path below the drive, slash-separated
}

// Plan computes the vault destination for the real path p (absolute,
// existing). os_specific forces the .os/<goos>/ layer. For a Windows
// path on a non-system drive, the returned route has NeedsVolume set
// and must be completed with WithVolume; rigo never guesses the
// volume.
func Plan(cfg *config.Config, h Host, volumes map[string]string, p string, os_specific bool) (Route, error) {
	p = filepath.Clean(p)
	if strings.HasPrefix(p, `\\`) {
		return Route{}, fmt.Errorf("%s: UNC paths are not supported", p)
	}

	os_prefix := cfg.OSDir + "/" + h.GOOS + "/"

	// Windows profile sections live inside the home directory, so they
	// must be checked first; they are inherently OS-specific.
	if h.GOOS == "windows" {
		if rel, ok := under(h.AppData, p); ok {
			return Route{Logical: ".appdata/" + rel, VaultRel: os_prefix + ".appdata/" + rel}, nil
		}
		if rel, ok := under(h.LocalAppData, p); ok {
			return Route{Logical: ".local/" + rel, VaultRel: os_prefix + ".local/" + rel}, nil
		}
	}

	logical := ""
	if rel, ok := under(h.ConfigHome, p); ok {
		logical = ".config/" + rel
	} else if rel, ok := under(h.Home, p); ok {
		logical = rel
	}
	if logical != "" {
		if os_specific {
			return Route{Logical: logical, VaultRel: os_prefix + logical}, nil
		}
		return Route{Logical: logical, VaultRel: logical}, nil
	}

	// Outside home: always .abs, inherently OS-specific.
	if h.GOOS != "windows" {
		rest := strings.TrimPrefix(filepath.ToSlash(p), "/")
		return Route{Logical: p, VaultRel: os_prefix + cfg.AbsDir + "/" + rest}, nil
	}

	// filepath.VolumeName only understands drive letters when compiled
	// for Windows, so parse the letter by hand (host-injected tests
	// exercise this code on every platform).
	drive := drive_of(p)
	if drive == "" {
		return Route{}, fmt.Errorf("%s: absolute Windows paths must carry a drive letter", p)
	}
	rest := strings.TrimPrefix(filepath.ToSlash(p[2:]), "/")
	r := Route{Drive: drive, rest: rest}
	if drive == system_letter(h) {
		return r.WithVolume(cfg, h, "system"), nil
	}
	r.NeedsVolume = true
	r.Suggest = suggest_volume(volumes, drive)
	return r, nil
}

// WithVolume completes a Windows absolute route with the chosen volume.
func (r Route) WithVolume(cfg *config.Config, h Host, name string) Route {
	r.Logical = name + ":/" + r.rest
	r.VaultRel = cfg.OSDir + "/" + h.GOOS + "/" + cfg.AbsDir + "/" + name + "/" + r.rest
	r.NeedsVolume = false
	return r
}

// suggest_volume picks the interactive default: the unique declared
// volume already resolving to the drive, otherwise "data".
func suggest_volume(volumes map[string]string, drive string) string {
	match := ""
	for name, letter := range volumes {
		if letter != drive {
			continue
		}
		if match != "" {
			return "data" // ambiguous: no single candidate
		}
		match = name
	}
	if match == "" {
		return "data"
	}
	return match
}

// drive_of extracts a lowercase drive letter from a Windows path.
func drive_of(p string) string {
	if len(p) >= 2 && p[1] == ':' &&
		(('a' <= p[0] && p[0] <= 'z') || ('A' <= p[0] && p[0] <= 'Z')) {
		return strings.ToLower(p[:1])
	}
	return ""
}

// under reports the slash-separated path of p relative to base.
func under(base, p string) (string, bool) {
	if base == "" {
		return "", false
	}
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// Volumes exposes the volume resolution for this host (used by the
// add command to route and validate --volume).
func Volumes(cfg *config.Config, host Host) (map[string]string, error) {
	return resolve_volumes(cfg, host)
}

// Ignored reports whether the vault-relative path would be invisible
// to scanning; adding such a path would create an unmanageable entry.
func Ignored(cfg *config.Config, vault_rel string, is_dir bool) (bool, error) {
	ig, err := new_ignorer(cfg.Ignore)
	if err != nil {
		return false, err
	}
	// Check every ancestor: a file inside an ignored directory is
	// itself invisible.
	parts := strings.Split(vault_rel, "/")
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		dir := is_dir || i < len(parts)
		if ig.match(prefix, dir) {
			return true, nil
		}
	}
	return false, nil
}

// Covered returns the directory-unit entry that contains the logical
// path, if any.
func Covered(entries []Entry, logical string) (Entry, bool) {
	want := norm(logical)
	for _, e := range entries {
		if e.Dir && (want == e.Path || strings.HasPrefix(want, e.Path+"/")) {
			return e, true
		}
	}
	return Entry{}, false
}
