package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rinodrops/rigo/internal/config"
)

func TestPlanRouting(t *testing.T) {
	cfg := load_config(t, `[volumes]
data = "d"`)
	home := filepath.FromSlash("/Users/rino")
	mac := Host{Name: "mac", GOOS: "darwin", Home: home}
	win := Host{
		Name: "winpc", GOOS: "windows",
		Home:         filepath.FromSlash("C:/Users/rino"),
		AppData:      filepath.FromSlash("C:/Users/rino/AppData/Roaming"),
		LocalAppData: filepath.FromSlash("C:/Users/rino/AppData/Local"),
		SysDrive:     "C:",
	}
	volumes := map[string]string{"system": "c", "data": "d"}

	cases := []struct {
		name, path, flavour string
		host                Host
		os_flag             bool
		logical, vaultrel   string
	}{
		{"home mirror", filepath.FromSlash("/Users/rino/.zshrc"), "", mac, false, ".zshrc", ".zshrc"},
		{"os layer", filepath.FromSlash("/Users/rino/.hammerspoon"), "", mac, true, ".hammerspoon", ".os/darwin/.hammerspoon"},
		{"flavour layer", filepath.FromSlash("/Users/rino/.zshrc"), "wsl",
			Host{Name: "box", GOOS: "linux", Home: home}, false,
			".zshrc", ".os/linux/.flavour/wsl/.zshrc"},
		{"flavour abs", filepath.FromSlash("/etc/hosts"), "wsl",
			Host{Name: "box", GOOS: "linux", Home: home}, false,
			filepath.FromSlash("/etc/hosts"), ".os/linux/.flavour/wsl/.abs/etc/hosts"},
		{"unix abs", filepath.FromSlash("/etc/hosts"), "", mac, false, filepath.FromSlash("/etc/hosts"), ".os/darwin/.abs/etc/hosts"},
		{"windows appdata", filepath.FromSlash("C:/Users/rino/AppData/Roaming/Code/settings.json"), "", win, false,
			".appdata/Code/settings.json", ".os/windows/.appdata/Code/settings.json"},
		{"windows localappdata", filepath.FromSlash("C:/Users/rino/AppData/Local/tool/t.ini"), "", win, false,
			".local/tool/t.ini", ".os/windows/.local/tool/t.ini"},
		{"windows home", filepath.FromSlash("C:/Users/rino/.nyagos"), "", win, true, ".nyagos", ".os/windows/.nyagos"},
		{"windows system drive", filepath.FromSlash("C:/Program Files/App/app.ini"), "", win, false,
			"system:/Program Files/App/app.ini", ".os/windows/.abs/system/Program Files/App/app.ini"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Plan(cfg, tc.host, volumes, tc.path, tc.os_flag, tc.flavour)
			if err != nil {
				t.Fatal(err)
			}
			if r.NeedsVolume {
				t.Fatalf("unexpected NeedsVolume: %+v", r)
			}
			if r.Logical != tc.logical || r.VaultRel != tc.vaultrel {
				t.Errorf("got (%q, %q), want (%q, %q)", r.Logical, r.VaultRel, tc.logical, tc.vaultrel)
			}
		})
	}
	if _, err := Plan(cfg, mac, volumes, filepath.FromSlash("/Users/rino/.zshrc"), false, "nope"); err == nil {
		t.Fatal("unknown flavour should fail")
	}
}

func TestPlanNonSystemDrive(t *testing.T) {
	cfg := load_config(t, "")
	win := Host{Name: "winpc", GOOS: "windows", Home: filepath.FromSlash("C:/Users/rino"), SysDrive: "C:"}

	// With a unique declared volume on that drive, it is the suggestion.
	r, err := Plan(cfg, win, map[string]string{"system": "c", "tools": "d"}, filepath.FromSlash("D:/Tools/foo.ini"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !r.NeedsVolume || r.Drive != "d" || r.Suggest != "tools" {
		t.Fatalf("route: %+v", r)
	}
	r = r.WithVolume(cfg, win, "tools")
	if r.Logical != "tools:/Tools/foo.ini" || r.VaultRel != ".os/windows/.abs/tools/Tools/foo.ini" {
		t.Errorf("completed route: %+v", r)
	}

	// Without a candidate the suggestion falls back to "data".
	r, err = Plan(cfg, win, map[string]string{"system": "c"}, filepath.FromSlash("D:/Tools/foo.ini"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Suggest != "data" {
		t.Errorf("suggest: %q", r.Suggest)
	}

	if _, err := Plan(cfg, win, nil, `\\server\share\x`, false, ""); err == nil || !strings.Contains(err.Error(), "UNC") {
		t.Errorf("want UNC error, got %v", err)
	}
}

func TestAddFile(t *testing.T) {
	e := file_entry(t)
	// file_entry writes the vault side; recreate as an add scenario:
	// content starts at the target, the vault side is empty.
	if err := os.Rename(e.Vault, e.Target); err != nil {
		t.Fatal(err)
	}

	if err := Add(e, false); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, e.Vault); got != "content\n" {
		t.Errorf("vault content: %q", got)
	}
}

func TestAddDir(t *testing.T) {
	e := dir_entry(t)
	if err := os.RemoveAll(e.Vault); err != nil {
		t.Fatal(err)
	}
	make_tree(t, e.Target, map[string]string{"vimrc": "set nu\n"})

	if err := Add(e, false); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, filepath.Join(e.Vault, "vimrc")); got != "set nu\n" {
		t.Errorf("vault content: %q", got)
	}
}

func TestAddAdoptsSymlinkReferent(t *testing.T) {
	e := file_entry(t)
	referent := filepath.Join(t.TempDir(), "real.conf")
	if err := os.Rename(e.Vault, referent); err != nil {
		t.Fatal(err)
	}
	symlink(t, referent, e.Target)

	if err := Add(e, false); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, e.Vault); got != "content\n" {
		t.Errorf("vault content: %q", got)
	}
	// Copy semantics: the referent survives untouched.
	if got := read(t, referent); got != "content\n" {
		t.Errorf("referent: %q", got)
	}
}

func TestAddKeepSymlink(t *testing.T) {
	e := file_entry(t)
	referent := filepath.Join(t.TempDir(), "real.conf")
	if err := os.Rename(e.Vault, referent); err != nil {
		t.Fatal(err)
	}
	symlink(t, referent, e.Target)

	if err := Add(e, true); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(e.Vault)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("vault side should store the symlink itself")
	}
}

func forget_fixture(t *testing.T) (Entry, string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{".zshrc": "content\n"})
	cfg := load_config(t, "")
	e := Entry{Path: ".zshrc", Vault: filepath.Join(root, ".zshrc"), Target: filepath.Join(home, ".zshrc")}
	return e, root, cfg
}

func expect_trashed(t *testing.T, root, trash_rel string) {
	t.Helper()
	if !strings.HasPrefix(trash_rel, ".trash/") || !strings.HasSuffix(trash_rel, "/.zshrc") {
		t.Errorf("trash location: %q", trash_rel)
	}
	if got := read(t, filepath.Join(root, filepath.FromSlash(trash_rel))); got != "content\n" {
		t.Errorf("trashed content: %q", got)
	}
}

func TestForgetLinked(t *testing.T) {
	e, root, cfg := forget_fixture(t)
	symlink(t, e.Vault, e.Target)

	trash_rel, err := Forget(e, root, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	expect_trashed(t, root, trash_rel)
	// The local file is a materialized real copy now.
	if fi, err := os.Lstat(e.Target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target should be a real file: %v %v", fi, err)
	}
	if got := read(t, e.Target); got != "content\n" {
		t.Errorf("materialized content: %q", got)
	}
	if _, err := os.Lstat(e.Vault); !os.IsNotExist(err) {
		t.Error("vault entry should be gone")
	}
}

func TestForgetConflictKeepsLocal(t *testing.T) {
	e, root, cfg := forget_fixture(t)
	if err := os.WriteFile(e.Target, []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	trash_rel, err := Forget(e, root, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	expect_trashed(t, root, trash_rel)
	if got := read(t, e.Target); got != "local edit\n" {
		t.Errorf("local content must survive: %q", got)
	}
}

func TestForgetPendingAndExcluded(t *testing.T) {
	for name, deployed := range map[string]bool{"pending": true, "excluded": false} {
		t.Run(name, func(t *testing.T) {
			e, root, cfg := forget_fixture(t)
			trash_rel, err := Forget(e, root, cfg, deployed)
			if err != nil {
				t.Fatal(err)
			}
			expect_trashed(t, root, trash_rel)
			if _, err := os.Lstat(e.Target); !os.IsNotExist(err) {
				t.Error("nothing should be materialized")
			}
		})
	}
}
