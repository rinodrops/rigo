package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rinodrops/rigo/internal/config"
)

// forget_entry forgets a vault path and returns the trash list.
func forget_entry(t *testing.T, root, home string, cfg *config.Config, rel string, dir bool) []TrashEntry {
	t.Helper()
	e := Entry{
		Path:   rel,
		Vault:  filepath.Join(root, filepath.FromSlash(rel)),
		Target: filepath.Join(home, filepath.FromSlash(rel)),
		Dir:    dir,
	}
	if _, err := Forget(e, root, cfg, false); err != nil {
		t.Fatal(err)
	}
	trash, err := TrashList(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}
	return trash
}

func TestTrashListAndRestore(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	make_tree(t, root, map[string]string{".config/tool/conf.toml": "x\n"})

	trash := forget_entry(t, root, home, cfg, ".config/tool/conf.toml", false)
	if len(trash) != 1 {
		t.Fatalf("trash: %+v", trash)
	}
	got := trash[0]
	if got.VaultRel != ".config/tool/conf.toml" || got.Logical != ".config/tool/conf.toml" {
		t.Errorf("entry: %+v", got)
	}
	if got.Target != filepath.Join(home, ".config", "tool", "conf.toml") {
		t.Errorf("target: %q", got.Target)
	}

	if err := TrashRestore(root, cfg, got); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, ".config", "tool", "conf.toml")); got != "x\n" {
		t.Errorf("restored vault content: %q", got)
	}
	// The emptied generation disappears from the listing.
	trash2, err := TrashList(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(trash2) != 0 {
		t.Errorf("trash after restore: %+v", trash2)
	}
}

func TestTrashRestoreRefusesOccupied(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	make_tree(t, root, map[string]string{".zshrc": "old\n"})
	trash := forget_entry(t, root, home, cfg, ".zshrc", false)
	make_tree(t, root, map[string]string{".zshrc": "new\n"}) // re-added

	if err := TrashRestore(root, cfg, trash[0]); err == nil || !strings.Contains(err.Error(), "already contains") {
		t.Errorf("want occupied error, got %v", err)
	}
}

func TestTrashNewestPicksLatest(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	make_tree(t, root, map[string]string{".zshrc": "first\n"})
	forget_entry(t, root, home, cfg, ".zshrc", false)
	make_tree(t, root, map[string]string{".zshrc": "second\n"})
	trash := forget_entry(t, root, home, cfg, ".zshrc", false)

	if len(trash) != 2 {
		t.Fatalf("trash: %+v", trash)
	}
	got, ok := Newest(trash, ".zshrc")
	if !ok {
		t.Fatal("no newest entry")
	}
	if content := read(t, got.Content); content != "second\n" {
		t.Errorf("newest content: %q", content)
	}
}

func TestGenerationEntryFallback(t *testing.T) {
	// A generation without metadata (pre-metadata trash) still lists.
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	make_tree(t, root, map[string]string{
		".trash/20260101T000000Z/.vim/vimrc":      "v\n",
		".trash/20260101T000000Z/.vim/colors/x":   "c\n",
		".trash/20260102T000000Z/.config/a/b.txt": "f\n",
	})
	trash, err := TrashList(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 2 {
		t.Fatalf("trash: %+v", trash)
	}
	// Newest first: the single-file chain resolves to the file, the
	// branching directory resolves to the directory itself.
	if trash[0].VaultRel != ".config/a/b.txt" {
		t.Errorf("file entry: %+v", trash[0])
	}
	if trash[1].VaultRel != ".vim" {
		t.Errorf("dir entry: %+v", trash[1])
	}
}

func TestDeriveMapping(t *testing.T) {
	cfg := load_config(t, `distros = ["ubuntu"]
[volumes]
data = "d"`)
	win := Host{Name: "winpc", GOOS: "windows", Home: filepath.FromSlash("C:/Users/rino"),
		AppData: filepath.FromSlash("C:/Users/rino/AppData/Roaming"), SysDrive: "C:"}
	linux := Host{Name: "box", GOOS: "linux", Distro: "ubuntu", Home: "/home/rino"}
	volumes := map[string]string{"system": "c", "data": "d"}

	cases := []struct {
		host              Host
		vault_rel, wantlg string
	}{
		{linux, ".zshrc", ".zshrc"},
		{linux, ".os/linux/.gitconfig", ".gitconfig"},
		{linux, ".os/linux/ubuntu/.vimrc", ".vimrc"},
		{linux, ".os/linux/.abs/etc/foo", "/etc/foo"},
		{linux, ".os/darwin/.hammerspoon/a", ""}, // another OS
		{win, ".os/windows/.appdata/Code/settings.json", ".appdata/Code/settings.json"},
		{win, ".os/windows/.abs/data/Tools/foo.ini", "data:/Tools/foo.ini"},
		{win, ".os/windows/.abs/orphan/x", ""}, // unresolved volume
	}
	for _, tc := range cases {
		logical, target := derive(cfg, tc.host, volumes, tc.vault_rel)
		if logical != tc.wantlg {
			t.Errorf("%s on %s: logical %q, want %q", tc.vault_rel, tc.host.GOOS, logical, tc.wantlg)
		}
		if (logical == "") != (target == "") {
			t.Errorf("%s: logical/target mismatch: %q %q", tc.vault_rel, logical, target)
		}
	}
}

func TestTrashEmptyOlderThan(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	old_stamp := time.Now().UTC().Add(-40 * 24 * time.Hour).Format("20060102T150405Z")
	make_tree(t, root, map[string]string{
		".trash/" + old_stamp + "/.old": "o\n",
	})
	make_tree(t, root, map[string]string{".zshrc": "z\n"})
	forget_entry(t, root, home, cfg, ".zshrc", false) // fresh generation

	deleted, err := TrashEmpty(root, cfg, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != old_stamp {
		t.Errorf("deleted: %v", deleted)
	}

	deleted, err = TrashEmpty(root, cfg, 0) // everything
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Errorf("deleted: %v", deleted)
	}
}

func TestFindStalePropagatedForget(t *testing.T) {
	// Machine B: the vault entry vanished (forgotten on machine A and
	// synced away), leaving a dangling symlink and a trash entry.
	root := t.TempDir()
	home := t.TempDir()
	cfg := load_config(t, "")
	make_tree(t, root, map[string]string{".zshrc": "z\n"})
	e := Entry{Path: ".zshrc", Vault: filepath.Join(root, ".zshrc"), Target: filepath.Join(home, ".zshrc")}
	symlink(t, e.Vault, e.Target) // deployed on machine B

	// Simulate machine A's forget arriving via sync: vault side moves
	// into the trash, the local symlink stays behind, dangling.
	if _, err := Forget(Entry{Path: e.Path, Vault: e.Vault, Target: filepath.Join(t.TempDir(), "elsewhere")}, root, cfg, false); err != nil {
		t.Fatal(err)
	}

	entries, _, err := Scan(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("vault should be empty: %v", paths(entries))
	}
	trash, err := TrashList(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}

	stale, err := FindStale(entries, trash, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Logical != ".zshrc" || !stale[0].HasCopy {
		t.Fatalf("stale: %+v", stale)
	}

	// r) restores the trash copy locally as a real file; the trash
	// copy itself stays for other machines.
	if err := RestoreLocal(stale[0].Trash); err != nil {
		t.Fatal(err)
	}
	if got := read(t, e.Target); got != "z\n" {
		t.Errorf("restored content: %q", got)
	}
	if fi, err := os.Lstat(e.Target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Error("target should be a real file")
	}
	if _, err := os.Stat(stale[0].Trash.Content); err != nil {
		t.Error("trash copy must survive a local restore")
	}
}

func TestRemoveLinkRefusesRealFiles(t *testing.T) {
	target := filepath.Join(t.TempDir(), "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLink(target); err == nil || !strings.Contains(err.Error(), "not a symlink") {
		t.Errorf("want refusal, got %v", err)
	}
}
