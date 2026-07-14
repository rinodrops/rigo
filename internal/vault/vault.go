// Package vault scans the vault tree, maps entries to their targets on
// this machine, evaluates per-machine selection, and detects entry
// states.
package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Host describes the machine entries are mapped onto. Fields are
// injected so that mapping rules for any OS can be tested anywhere.
type Host struct {
	Name         string // short hostname: up to the first dot, lowercased
	GOOS         string
	Distro       string // /etc/os-release ID; empty outside Linux
	Home         string
	ConfigHome   string // $XDG_CONFIG_HOME; empty when unset
	AppData      string // Windows %APPDATA%
	LocalAppData string // Windows %LOCALAPPDATA%
	SysDrive     string // Windows %SystemDrive%, e.g. "C:"
}

// Current builds the Host for the running machine.
func Current() (Host, error) {
	name, err := os.Hostname()
	if err != nil {
		return Host{}, err
	}
	name, _, _ = strings.Cut(name, ".")
	home, err := os.UserHomeDir()
	if err != nil {
		return Host{}, err
	}
	h := Host{
		Name:         strings.ToLower(name),
		GOOS:         runtime.GOOS,
		Home:         home,
		ConfigHome:   os.Getenv("XDG_CONFIG_HOME"),
		AppData:      os.Getenv("APPDATA"),
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		SysDrive:     os.Getenv("SystemDrive"),
	}
	if h.GOOS == "linux" {
		h.Distro = os_release_id("/etc/os-release")
	}
	if h.GOOS == "windows" && h.SysDrive == "" {
		h.SysDrive = "C:"
	}
	return h, nil
}

// os_release_id extracts the ID field (machine-readable, lowercase)
// from an os-release file. Missing file or field yields "".
func os_release_id(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "ID="); ok {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

// Entry is one managed unit: a file, or a whole directory declared for
// directory-unit deployment.
type Entry struct {
	Path   string // logical path: home-relative (slash-separated), or absolute for .abs entries
	Vault  string // absolute path of the vault source
	Target string // absolute path the entry deploys to on this machine
	Dir    bool   // directory-unit entry
}

// Target maps a home-relative logical path to its location on this
// host. Secrets share this mapping: their declared paths are
// home-relative like ordinary entries.
func (h Host) Target(logical string) string {
	return h.target(logical, false)
}

// target maps a logical path from one vault layer to its absolute
// location on this host. abs is true inside a .abs subtree, where the
// logical path mirrors the filesystem root (Windows .abs paths go
// through named volumes in the scanner instead).
func (h Host) target(logical string, abs bool) string {
	if abs {
		return filepath.Join("/", filepath.FromSlash(logical))
	}
	if rest, ok := strings.CutPrefix(logical, ".config/"); ok && h.ConfigHome != "" {
		return filepath.Join(h.ConfigHome, filepath.FromSlash(rest))
	}
	return filepath.Join(h.Home, filepath.FromSlash(logical))
}

// win_section rewrites the first path element of a logical path inside
// .os/windows/: .appdata/ and .local/ map to their profile dirs and
// are stripped from the logical remainder handled by target().
func (h Host) win_target(logical string) string {
	if rest, ok := strings.CutPrefix(logical, ".appdata/"); ok {
		return filepath.Join(h.AppData, filepath.FromSlash(rest))
	}
	if rest, ok := strings.CutPrefix(logical, ".local/"); ok {
		return filepath.Join(h.LocalAppData, filepath.FromSlash(rest))
	}
	return h.target(logical, false)
}
