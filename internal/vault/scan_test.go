package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rinodrops/rigo/internal/config"
)

// make_tree materializes a file tree: keys ending in "/" become
// directories, others files with their value as content.
func make_tree(t *testing.T, root string, tree map[string]string) {
	t.Helper()
	for rel, content := range tree {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func load_config(t *testing.T, content string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rigo.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func darwin_host(home string) Host {
	return Host{Name: "mac", GOOS: "darwin", Home: home}
}

// paths extracts the sorted logical paths from entries.
func paths(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func find(t *testing.T, entries []Entry, p string) Entry {
	t.Helper()
	e, ok := Find(entries, p)
	if !ok {
		t.Fatalf("entry %q not found in %v", p, paths(entries))
	}
	return e
}

func TestScanBasic(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{
		".zshrc":                 "z",
		".config/rigo/rigo.toml": "c",
		".vim/vimrc":             "v", // dir-unit via dirs
		".zsh/aliases.zsh":       "a", // dir-unit via tag
		".claude/settings.json":  "s", // container
		".claude/local.json":     "l",
		".DS_Store":              "junk",
		".claude/.DS_Store":      "junk",
		"note.bak":               "junk",
		"proj/node_modules/x.js": "junk",
		"._.zshrc":               "junk", // AppleDouble sidecar
		".zshrc.icloud":          "junk", // iCloud placeholder
		".git/config":            "junk", // vault under version control
		"@eaDir/thumb.jpg":       "junk", // Synology
		".claude/settings (rino's conflicted copy 2026-07-13).json": "junk",
		".trash/20260101T000000/":                                   "",
		".os/linux/.vimrc":                                          "other-os",
	})
	cfg := load_config(t, `
dirs   = [".vim/"]
ignore = ["*.bak", "**/node_modules/"]
[tags]
zsh = [".zsh/", ".zshrc"]
`)

	entries, warnings, err := Scan(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	want := []string{
		".claude/local.json", ".claude/settings.json",
		".config/rigo/rigo.toml", ".vim", ".zsh", ".zshrc",
	}
	got := paths(entries)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("entries: got %v, want %v", got, want)
	}
	if e := find(t, entries, ".vim"); !e.Dir {
		t.Error(".vim should be a directory-unit entry")
	}
	if e := find(t, entries, ".zsh/"); !e.Dir {
		t.Error(".zsh should be a directory-unit entry (declared in a tag)")
	}
	if e := find(t, entries, ".zshrc"); e.Dir || e.Target != filepath.Join(home, ".zshrc") {
		t.Errorf(".zshrc: %+v", e)
	}
}

func TestScanOSOverlayAndDistro(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{
		".gitconfig":                 "common",
		".os/linux/.gitconfig":       "linux",
		".os/linux/.vimrc":           "linux",
		".os/linux/ubuntu/.vimrc":    "ubuntu",
		".os/linux/debian/.vimrc":    "debian",
		".os/darwin/.hammerspoon/a":  "mac-only",
		".os/linux/.abs/etc/foo.cfg": "abs",
		".os/linux/opt/tool.conf":    "non-dot content",
	})
	cfg := load_config(t, `distros = ["ubuntu", "debian"]`)
	host := Host{Name: "box", GOOS: "linux", Distro: "ubuntu", Home: home}

	entries, _, err := Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitconfig", ".vimrc", "/etc/foo.cfg", "opt/tool.conf"}
	if got := paths(entries); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("entries: got %v, want %v", got, want)
	}
	if e := find(t, entries, ".gitconfig"); !strings.Contains(e.Vault, filepath.FromSlash(".os/linux/")) {
		t.Errorf(".gitconfig should come from the linux layer, got %s", e.Vault)
	}
	if e := find(t, entries, ".vimrc"); !strings.Contains(e.Vault, "ubuntu") {
		t.Errorf(".vimrc should come from the ubuntu overlay, got %s", e.Vault)
	}
	if e := find(t, entries, "/etc/foo.cfg"); e.Target != filepath.Join("/", "etc", "foo.cfg") {
		t.Errorf("abs target: %+v", e)
	}
}

func TestScanWindowsSections(t *testing.T) {
	root := t.TempDir()
	make_tree(t, root, map[string]string{
		".os/windows/.nyagos":                    "profile",
		".os/windows/.config/tool/conf":          "xdg",
		".os/windows/.appdata/Code/settings":     "appdata",
		".os/windows/.local/Programs/tool/t.ini": "localappdata",
		".os/windows/.abs/Program Files/x/x.ini": "abs",
	})
	cfg := load_config(t, "")
	host := Host{
		Name: "winpc", GOOS: "windows",
		Home:         filepath.FromSlash("C:/Users/rino"),
		AppData:      filepath.FromSlash("C:/Users/rino/AppData/Roaming"),
		LocalAppData: filepath.FromSlash("C:/Users/rino/AppData/Local"),
		SysDrive:     "C:",
	}

	entries, _, err := Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		".nyagos":                    filepath.Join(host.Home, ".nyagos"),
		".config/tool/conf":          filepath.Join(host.Home, ".config", "tool", "conf"),
		".appdata/Code/settings":     filepath.Join(host.AppData, "Code", "settings"),
		".local/Programs/tool/t.ini": filepath.Join(host.LocalAppData, "Programs", "tool", "t.ini"),
		"/Program Files/x/x.ini":     filepath.Join("C:"+string(filepath.Separator), "Program Files", "x", "x.ini"),
	}
	for logical, want := range cases {
		if e := find(t, entries, logical); e.Target != want {
			t.Errorf("%s: target %q, want %q", logical, e.Target, want)
		}
	}
}

func TestScanXDGConfigHome(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	config_home := t.TempDir()
	make_tree(t, root, map[string]string{".config/rigo/rigo.toml": "c", ".zshrc": "z"})
	cfg := load_config(t, "")
	host := darwin_host(home)
	host.ConfigHome = config_home

	entries, _, err := Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	if e := find(t, entries, ".config/rigo/rigo.toml"); e.Target != filepath.Join(config_home, "rigo", "rigo.toml") {
		t.Errorf("XDG target: %q", e.Target)
	}
	if e := find(t, entries, ".zshrc"); e.Target != filepath.Join(home, ".zshrc") {
		t.Errorf("home target: %q", e.Target)
	}
}

func TestScanGuards(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name, toml string
		tree       map[string]string
		want       string
	}{
		{
			"slashed file",
			`dirs = [".vim/"]` + "\n[tags]\nv = [\".vimrc/\"]",
			map[string]string{".vimrc": "x", ".vim/rc": "y"},
			"trailing slash",
		},
		{
			"file in dirs",
			`dirs = [".vimrc"]`,
			map[string]string{".vimrc": "x"},
			"may only name directories",
		},
		{
			"declaration under dir-unit",
			"dirs = [\".vim/\"]\n[tags]\nv = [\".vim/vimrc\"]",
			map[string]string{".vim/vimrc": "x"},
			"single directory symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			make_tree(t, root, tc.tree)
			_, _, err := Scan(root, load_config(t, tc.toml), darwin_host(home))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func TestScanWarnsUndeclaredDistro(t *testing.T) {
	root := t.TempDir()
	make_tree(t, root, map[string]string{".os/linux/ubuntu/.vimrc": "u"})
	cfg := load_config(t, "")
	host := Host{Name: "box", GOOS: "linux", Distro: "ubuntu", Home: t.TempDir()}

	entries, warnings, err := Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "not declared in distros") {
		t.Errorf("warnings: %v", warnings)
	}
	// Without the declaration the directory is plain home content.
	if _, ok := Find(entries, "ubuntu/.vimrc"); !ok {
		t.Errorf("undeclared distro dir should scan as content, got %v", paths(entries))
	}
}

func TestScanWarnsUndeclared(t *testing.T) {
	root := t.TempDir()
	make_tree(t, root, map[string]string{".zshrc": "z"})
	cfg := load_config(t, `dirs = [".vim/"]`)

	_, warnings, err := Scan(root, cfg, darwin_host(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], ".vim") {
		t.Errorf("warnings: %v", warnings)
	}
}
