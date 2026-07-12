package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// make_vault creates a vault containing .config/rigo/rigo.toml and
// returns the vault root and the config path inside it.
func make_vault(t *testing.T) (string, string) {
	t.Helper()
	vault := filepath.Join(t.TempDir(), "vault")
	cfg := filepath.Join(vault, filepath.FromSlash(config_rel))
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("dirs = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return vault, cfg
}

// link_config symlinks <config_home>/rigo/rigo.toml to target and sets
// XDG_CONFIG_HOME accordingly. Skips the test where symlinks need
// privileges (Windows without Developer Mode).
func link_config(t *testing.T, target string) string {
	t.Helper()
	config_home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config_home)
	link := filepath.Join(config_home, "rigo", "rigo.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	return link
}

func TestDiscover(t *testing.T) {
	vault, cfg := make_vault(t)
	link_config(t, cfg)

	got_cfg, got_vault, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if got_cfg != cfg || got_vault != vault {
		t.Errorf("got (%q, %q), want (%q, %q)", got_cfg, got_vault, cfg, vault)
	}
}

func TestDiscoverMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, _, err := Discover()
	if err == nil || !strings.Contains(err.Error(), "-f") {
		t.Errorf("want a bootstrap hint mentioning -f, got %v", err)
	}
}

func TestDiscoverRegularFile(t *testing.T) {
	config_home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config_home)
	path := filepath.Join(config_home, "rigo", "rigo.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("dirs = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := Discover()
	if err == nil || !strings.Contains(err.Error(), "not a symlink") {
		t.Errorf("want a not-a-symlink error, got %v", err)
	}
}

func TestDiscoverBrokenLink(t *testing.T) {
	vault, cfg := make_vault(t)
	link_config(t, cfg)
	if err := os.RemoveAll(vault); err != nil {
		t.Fatal(err)
	}

	_, _, err := Discover()
	if err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Errorf("want a broken-target error, got %v", err)
	}
}

func TestDiscoverForeignTarget(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "rigo.toml")
	if err := os.WriteFile(outside, []byte("dirs = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link_config(t, outside)

	_, _, err := Discover()
	if err == nil || !strings.Contains(err.Error(), "does not point into a vault") {
		t.Errorf("want a foreign-target error, got %v", err)
	}
}

func TestDiscoverRelativeLink(t *testing.T) {
	// A relative symlink target must be resolved against the link's
	// own directory, not the process working directory.
	vault, cfg := make_vault(t)
	config_home := filepath.Join(filepath.Dir(vault), "confighome")
	t.Setenv("XDG_CONFIG_HOME", config_home)
	link := filepath.Join(config_home, "rigo", "rigo.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(filepath.Dir(link), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	_, got_vault, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if got_vault != vault {
		t.Errorf("got vault %q, want %q", got_vault, vault)
	}
}

func TestFromFile(t *testing.T) {
	vault, cfg := make_vault(t)

	got_cfg, got_vault, err := FromFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// t.TempDir may itself contain symlinks (e.g. /tmp on macOS), so
	// compare against the fully resolved expectation.
	want_vault, err := filepath.EvalSymlinks(vault)
	if err != nil {
		t.Fatal(err)
	}
	if got_vault != want_vault {
		t.Errorf("got vault %q, want %q", got_vault, want_vault)
	}
	if filepath.Base(got_cfg) != "rigo.toml" {
		t.Errorf("got config %q", got_cfg)
	}
}

func TestFromFileMissing(t *testing.T) {
	_, _, err := FromFile(filepath.Join(t.TempDir(), "rigo.toml"))
	if err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestVaultRootVolumeRoot(t *testing.T) {
	path := filepath.FromSlash("/.config/rigo/rigo.toml")
	if _, err := vault_root(path); err == nil || !strings.Contains(err.Error(), "volume root") {
		t.Errorf("want a volume-root error, got %v", err)
	}
}
