// Package secrets materializes password-manager secrets at their
// target paths. The source of truth is always the backend; rigo only
// fetches and writes.
package secrets

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

// Runner fetches one backend reference; injected for tests.
type Runner func(ref string) ([]byte, error)

// runner_for dispatches on the reference's URI scheme. Backends are
// external CLIs invoked as subprocesses; adding one here must never
// change how refs are written in rigo.toml.
func runner_for(ref string) (Runner, error) {
	scheme, _, ok := strings.Cut(ref, "://")
	if !ok {
		return nil, fmt.Errorf("%s has no backend scheme", ref)
	}
	switch scheme {
	case "op":
		return op_read, nil
	default:
		return nil, fmt.Errorf("no backend for scheme %q (known: op)", scheme)
	}
}

// op_read fetches a ref via the 1Password CLI. --no-newline keeps the
// value byte-exact (op appends a newline by default). Stdin and stderr
// pass through so 1Password's own auth prompts work untouched.
func op_read(ref string) ([]byte, error) {
	cmd := exec.Command("op", "read", "--no-newline", ref)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("op read %s: %w", ref, err)
	}
	return out, nil
}

// Item is one applicable secret on this host.
type Item struct {
	Path   string // home-relative logical path
	Ref    string
	Mode   fs.FileMode
	Target string // absolute destination
}

// Plan resolves the applicable secrets: os-filtered, selected on this
// host, sorted by path. A secrets path that is also a vault entry is
// an error, and an explicit only path must name a selected secret.
func Plan(cfg *config.Config, host vault.Host, sel *vault.Selection, entries []vault.Entry, only string) ([]Item, error) {
	var items []Item
	for path, sec := range cfg.Secrets {
		if !os_applies(sec.OS, host.GOOS) {
			continue
		}
		if _, ok := vault.Find(entries, path); ok {
			return nil, fmt.Errorf("%s is both a vault entry and a secret; a path cannot be deployed as a symlink and written as a secret", path)
		}
		items = append(items, Item{
			Path:   path,
			Ref:    sec.Ref,
			Mode:   sec.Mode,
			Target: host.Target(path),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })

	if only != "" {
		for _, item := range items {
			if item.Path != only {
				continue
			}
			if !sel.Selected(vault.Entry{Path: item.Path}) {
				return nil, fmt.Errorf("%s is excluded on this host", item.Path)
			}
			return []Item{item}, nil
		}
		return nil, fmt.Errorf("%s is not a secrets entry applicable on this OS", only)
	}

	selected := items[:0]
	for _, item := range items {
		if sel.Selected(vault.Entry{Path: item.Path}) {
			selected = append(selected, item)
		}
	}
	return selected, nil
}

func os_applies(list []string, goos string) bool {
	if len(list) == 0 {
		return true
	}
	for _, os_name := range list {
		if os_name == goos {
			return true
		}
	}
	return false
}

// Apply fetches the item and (over)writes its target: parent
// directories 0700, the file with the item's mode, atomically via a
// sibling temp file so an interrupted write never leaves a partial
// secret.
func Apply(run Runner, item Item) error {
	if run == nil {
		var err error
		if run, err = runner_for(item.Ref); err != nil {
			return err
		}
	}
	data, err := run(item.Ref)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(item.Target), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(item.Target), ".rigo-secret-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(item.Mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), item.Target)
}

// Applied reports whether the item's file exists (the only status the
// design allows: no hashes, no freshness).
func Applied(item Item) bool {
	_, err := os.Stat(item.Target)
	return err == nil
}

// Remove deletes the written file (the reverse of Apply). Removing an
// absent file is not an error; the bool reports whether anything was
// deleted.
func Remove(item Item) (bool, error) {
	err := os.Remove(item.Target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
