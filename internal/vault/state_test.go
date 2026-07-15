package vault

import (
	"os"
	"path/filepath"
	"testing"
)

// scenario builds a vault file entry and returns it with the paths
// prepared; the target is left for each test to arrange.
func file_entry(t *testing.T) Entry {
	t.Helper()
	vault_dir := t.TempDir()
	target_dir := t.TempDir()
	source := filepath.Join(vault_dir, ".zshrc")
	if err := os.WriteFile(source, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Entry{Path: ".zshrc", Vault: source, Target: filepath.Join(target_dir, ".zshrc")}
}

func dir_entry(t *testing.T) Entry {
	t.Helper()
	vault_dir := t.TempDir()
	target_dir := t.TempDir()
	source := filepath.Join(vault_dir, ".vim")
	make_tree(t, source, map[string]string{"vimrc": "set nu\n", "colors/x.vim": "hi\n"})
	return Entry{Path: ".vim", Vault: source, Target: filepath.Join(target_dir, ".vim"), Dir: true}
}

func expect_state(t *testing.T, e Entry, want State) {
	t.Helper()
	got, err := Detect(e)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("state: got %s, want %s", got, want)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

func TestDetectPending(t *testing.T) {
	expect_state(t, file_entry(t), Pending)
}

func TestDetectLinked(t *testing.T) {
	e := file_entry(t)
	symlink(t, e.Vault, e.Target)
	expect_state(t, e, Linked)
}

func TestDetectUnlinked(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Unlinked)
}

func TestDetectConflict(t *testing.T) {
	e := file_entry(t)
	if err := os.WriteFile(e.Target, []byte("different\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Conflict)
}

func TestDetectBrokenDangling(t *testing.T) {
	e := file_entry(t)
	symlink(t, filepath.Join(filepath.Dir(e.Vault), "gone"), e.Target)
	expect_state(t, e, Broken)
}

func TestDetectBrokenElsewhere(t *testing.T) {
	e := file_entry(t)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink(t, other, e.Target)
	expect_state(t, e, Broken)
}

func TestDetectDirLinked(t *testing.T) {
	e := dir_entry(t)
	symlink(t, e.Vault, e.Target)
	expect_state(t, e, Linked)
}

func TestDetectDirUnlinked(t *testing.T) {
	e := dir_entry(t)
	make_tree(t, e.Target, map[string]string{"vimrc": "set nu\n", "colors/x.vim": "hi\n"})
	expect_state(t, e, Unlinked)
}

func TestDetectDirConflict(t *testing.T) {
	cases := map[string]map[string]string{
		"changed file": {"vimrc": "set nonu\n", "colors/x.vim": "hi\n"},
		"extra file":   {"vimrc": "set nu\n", "colors/x.vim": "hi\n", "extra": ""},
		"missing file": {"vimrc": "set nu\n"},
	}
	for name, tree := range cases {
		t.Run(name, func(t *testing.T) {
			e := dir_entry(t)
			make_tree(t, e.Target, tree)
			expect_state(t, e, Conflict)
		})
	}
}

func TestDetectDirTargetIsFile(t *testing.T) {
	e := dir_entry(t)
	if err := os.WriteFile(e.Target, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect_state(t, e, Conflict)
}
