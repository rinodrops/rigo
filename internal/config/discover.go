package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// config_rel is where rigo.toml lives, both inside the vault and
// (as a symlink) under the user's config directory.
const config_rel = ".config/rigo/rigo.toml"

// Discover locates rigo.toml via the symlink at
// $XDG_CONFIG_HOME/rigo/rigo.toml (default ~/.config/rigo/rigo.toml)
// and derives the vault root from the symlink target. It returns the
// resolved config path and the vault root.
func Discover() (string, string, error) {
	config_home := os.Getenv("XDG_CONFIG_HOME")
	if config_home == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		config_home = filepath.Join(home, ".config")
	}
	link := filepath.Join(config_home, "rigo", "rigo.toml")

	fi, err := os.Lstat(link)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", fmt.Errorf(
			"no config found at %s; run \"rigo -f <vault>/%s <command>\" once to bootstrap",
			link, config_rel)
	}
	if err != nil {
		return "", "", err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return "", "", fmt.Errorf(
			"%s is not a symlink into the vault, so the vault location cannot be derived from it; re-run with \"rigo -f <vault>/%s <command>\"",
			link, config_rel)
	}

	target, err := os.Readlink(link)
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	target = filepath.Clean(target)
	if _, err := os.Stat(target); err != nil {
		return "", "", fmt.Errorf(
			"%s points to %s, which is not readable (vault not synced yet, or moved?): %w",
			link, target, err)
	}

	vault, err := vault_root(target)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", link, err)
	}
	return target, vault, nil
}

// ExpandHome expands a leading ~ (alone, ~/, or ~\) to the user's home
// directory. Shells that do not expand ~ for external commands
// (PowerShell among them) pass it through literally, so rigo handles it
// itself. ~user forms pass through unchanged.
func ExpandHome(arg string) (string, error) {
	if arg != "~" && !strings.HasPrefix(arg, "~/") && !strings.HasPrefix(arg, `~\`) {
		return arg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if arg == "~" {
		return home, nil
	}
	return filepath.Join(home, arg[2:]), nil
}

// FromFile derives the config path and vault root from an explicitly
// given rigo.toml path (the global -f flag).
func FromFile(path string) (string, string, error) {
	path, err := ExpandHome(path)
	if err != nil {
		return "", "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", err
	}
	vault, err := vault_root(real)
	if err != nil {
		return "", "", err
	}
	return real, vault, nil
}

// vault_root strips the .config/rigo/rigo.toml suffix from a resolved
// config path. The config always lives at that fixed location inside
// the vault, so anything else cannot be a vault.
func vault_root(path string) (string, error) {
	slashed := filepath.ToSlash(path)
	suffix := "/" + config_rel
	if !strings.HasSuffix(slashed, suffix) {
		return "", fmt.Errorf(
			"config path %s does not end with %s, so it does not point into a vault", path, config_rel)
	}
	vault := filepath.FromSlash(strings.TrimSuffix(slashed, suffix))
	if vault == "" {
		vault = string(filepath.Separator)
	}
	// A vault at a volume root would drown in OS-generated entries
	// (.Spotlight-V100, lost+found, $RECYCLE.BIN, ...).
	if filepath.Dir(vault) == vault {
		return "", fmt.Errorf("vault %s is a volume root, which is not supported", vault)
	}
	return vault, nil
}
