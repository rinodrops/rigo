package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLinkPending(t *testing.T) {
	e := file_entry(t)
	// The target's parent may be missing on a fresh machine.
	e.Target = filepath.Join(filepath.Dir(e.Target), "nested", "deep", ".zshrc")

	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
}

func TestLinkUnlinked(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
}

func TestLinkConflictKeepsVault(t *testing.T) {
	// Link on a conflict (the --force path) replaces the local file.
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, e.Target); got != "content\n" {
		t.Errorf("target content: %q", got)
	}
	if junk, _ := filepath.Glob(filepath.Join(filepath.Dir(e.Target), ".*rigo*")); len(junk) != 0 {
		t.Errorf("staging leftovers: %v", junk)
	}
}

func TestLinkDirUnit(t *testing.T) {
	e := dir_entry(t)
	make_tree(t, e.Target, map[string]string{"vimrc": "set nu\n", "colors/x.vim": "hi\n"})
	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
}

func TestUnlinkFile(t *testing.T) {
	e := file_entry(t)
	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	if err := Unlink(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Unlinked)
	if fi, err := os.Lstat(e.Target); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target should be a real file: %v %v", fi, err)
	}
	// The vault copy must survive.
	if got := read(t, e.Vault); got != "content\n" {
		t.Errorf("vault content: %q", got)
	}
}

func TestUnlinkDir(t *testing.T) {
	e := dir_entry(t)
	if err := Link(e); err != nil {
		t.Fatal(err)
	}
	if err := Unlink(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Unlinked)
	if got := read(t, filepath.Join(e.Target, "colors", "x.vim")); got != "hi\n" {
		t.Errorf("materialized tree content: %q", got)
	}
}

func TestAdoptFile(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Adopt(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, e.Vault); got != "local edit\n" {
		t.Errorf("vault should hold the adopted content: %q", got)
	}
}

func TestAdoptDir(t *testing.T) {
	e := dir_entry(t)
	make_tree(t, e.Target, map[string]string{"vimrc": "set nonu\n", "new.vim": "n\n"})
	if err := Adopt(e); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Linked)
	if got := read(t, filepath.Join(e.Vault, "vimrc")); got != "set nonu\n" {
		t.Errorf("vault vimrc: %q", got)
	}
	if got := read(t, filepath.Join(e.Vault, "new.vim")); got != "n\n" {
		t.Errorf("vault new.vim: %q", got)
	}
	if _, err := os.Stat(filepath.Join(e.Vault, "colors", "x.vim")); !os.IsNotExist(err) {
		t.Error("files absent locally should be gone from the vault after adoption")
	}
}

func TestApplyConverges(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{
		".zshrc":     "z\n", // pending
		".gitconfig": "g\n", // unlinked (identical local copy below)
		".netrc":     "n\n", // conflict
		".vimrc":     "v\n", // broken (dangling symlink below)
		".excluded":  "x\n",
	})
	make_tree(t, home, map[string]string{
		".gitconfig": "g\n",
		".netrc":     "different\n",
	})
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(home, ".vimrc")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	cfg := load_config(t, "[exclude]\nmac = [\".excluded\"]")
	entries, _, err := Scan(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}

	res := Apply(entries, Select(cfg, "mac"))
	if strings.Join(res.Linked, ",") != ".gitconfig,.zshrc" {
		t.Errorf("linked: %v", res.Linked)
	}
	if strings.Join(res.Conflicts, ",") != ".netrc" {
		t.Errorf("conflicts: %v", res.Conflicts)
	}
	if strings.Join(res.Broken, ",") != ".vimrc" {
		t.Errorf("broken: %v", res.Broken)
	}
	if res.Excluded != 1 {
		t.Errorf("excluded: %d", res.Excluded)
	}
	for _, p := range []string{".zshrc", ".gitconfig"} {
		st, err := Detect(Entry{Path: p, Vault: filepath.Join(root, p), Target: filepath.Join(home, p)})
		if err != nil || st != Linked {
			t.Errorf("%s: state %v err %v", p, st, err)
		}
	}
	// Conflict and broken targets are untouched.
	if got := read(t, filepath.Join(home, ".netrc")); got != "different\n" {
		t.Errorf(".netrc was touched: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".excluded")); !os.IsNotExist(err) {
		t.Error(".excluded should not be deployed")
	}
}

func TestApplyContinuesPastFailures(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{
		".blocker/inner.txt": "x\n", // will fail: parent path is a file locally
		".zshrc":             "z\n", // must still converge
	})
	// A real *file* where the entry's parent directory should be makes
	// MkdirAll fail — similar to a missing drive on Windows.
	make_tree(t, home, map[string]string{".blocker": "i am a file\n"})
	cfg := load_config(t, "")
	entries, _, err := Scan(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}

	res := Apply(entries, Select(cfg, "mac"))
	if len(res.Failed) != 1 || !strings.Contains(res.Failed[0], ".blocker/inner.txt") {
		t.Errorf("failed: %v", res.Failed)
	}
	if len(res.Linked) != 1 || res.Linked[0] != ".zshrc" {
		t.Errorf("linked: %v", res.Linked)
	}
}

func TestCompareFiles(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("content\nextra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Compare(e)
	if err != nil {
		t.Fatal(err)
	}
	if d.Stat != "+1 -0" {
		t.Errorf("stat: %q", d.Stat)
	}
	if !strings.Contains(d.Unified, "+extra") {
		t.Errorf("unified: %q", d.Unified)
	}
}

func TestCompareBinary(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Compare(e)
	if err != nil {
		t.Fatal(err)
	}
	if d.Stat != "binary files differ" || d.Unified != "" {
		t.Errorf("diff: %+v", d)
	}
}

func TestCompareTrees(t *testing.T) {
	e := dir_entry(t)
	make_tree(t, e.Target, map[string]string{"vimrc": "set nonu\n", "new.vim": "n\n"})
	d, err := Compare(e)
	if err != nil {
		t.Fatal(err)
	}
	if d.Stat != "1 added, 1 removed, 1 changed (local vs vault)" {
		t.Errorf("stat: %q", d.Stat)
	}
	for _, want := range []string{"+ new.vim", "- colors/x.vim", "~ vimrc"} {
		if !strings.Contains(d.Unified, want) {
			t.Errorf("summary misses %q:\n%s", want, d.Unified)
		}
	}
}
