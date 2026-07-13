package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const edit_fixture = `# my dotfiles
dirs = [".hammerspoon/"] # dir units

[tags]
# vim family
vim = [".vim/", ".vimrc"]

[exclude]
macbook = [".hammerspoon/", "vim"]
`

func open_fixture(t *testing.T, content string) *Edit {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rigo.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := OpenEdit(path)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func saved(t *testing.T, e *Edit) string {
	t.Helper()
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditRoundTripPreservesComments(t *testing.T) {
	e := open_fixture(t, edit_fixture)
	out := saved(t, e)
	for _, want := range []string{"# my dotfiles", "# dir units", "# vim family"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q:\n%s", want, out)
		}
	}
}

func TestEditAppend(t *testing.T) {
	e := open_fixture(t, edit_fixture)
	if err := e.AppendItem([]string{"dirs"}, ".zsh/"); err != nil {
		t.Fatal(err)
	}
	if err := e.AppendItem([]string{"tags", "vim"}, ".config/nvim/"); err != nil {
		t.Fatal(err)
	}
	if err := e.AppendItem([]string{"tags", "shell"}, ".zshrc"); err != nil {
		t.Fatal(err)
	}
	out := saved(t, e)

	cfg, err := Load(e.path)
	if err != nil {
		t.Fatalf("edited file no longer loads: %v\n%s", err, out)
	}
	if len(cfg.Dirs) != 2 || cfg.Dirs[1] != ".zsh/" {
		t.Errorf("dirs: %v", cfg.Dirs)
	}
	if got := cfg.Tags["vim"]; len(got) != 3 || got[2] != ".config/nvim/" {
		t.Errorf("tags.vim: %v", got)
	}
	if got := cfg.Tags["shell"]; len(got) != 1 || got[0] != ".zshrc" {
		t.Errorf("tags.shell: %v", got)
	}
	if !strings.Contains(out, "# vim family") {
		t.Errorf("lost comment:\n%s", out)
	}
}

func TestEditAppendIntoEmptyFile(t *testing.T) {
	e := open_fixture(t, "")
	if err := e.AppendItem([]string{"dirs"}, ".vim/"); err != nil {
		t.Fatal(err)
	}
	if err := e.SetKey([]string{"volumes", "data"}, "d"); err != nil {
		t.Fatal(err)
	}
	saved(t, e)

	cfg, err := Load(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Dirs) != 1 || cfg.Dirs[0] != ".vim/" {
		t.Errorf("dirs: %v", cfg.Dirs)
	}
	if cfg.Volumes["data"].Default != "d" {
		t.Errorf("volumes: %+v", cfg.Volumes)
	}
}

func TestEditSetKeyOverwrites(t *testing.T) {
	e := open_fixture(t, "[volumes]\ndata = \"d\"\n")
	if err := e.SetKey([]string{"volumes", "data"}, "e"); err != nil {
		t.Fatal(err)
	}
	saved(t, e)
	cfg, err := Load(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Volumes["data"].Default != "e" {
		t.Errorf("volumes: %+v", cfg.Volumes)
	}
}

func TestEditRemoveRefs(t *testing.T) {
	e := open_fixture(t, edit_fixture)
	// ".hammerspoon/" appears in dirs and in exclude.macbook; the
	// query uses no trailing slash on purpose.
	if got := e.RemoveRefs(".hammerspoon"); got != 2 {
		t.Errorf("removed %d, want 2", got)
	}
	out := saved(t, e)
	if strings.Contains(out, ".hammerspoon") {
		t.Errorf("reference survived:\n%s", out)
	}
	for _, want := range []string{"# my dotfiles", "# vim family", "dirs", "macbook"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q:\n%s", want, out)
		}
	}
	cfg, err := Load(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Dirs) != 0 {
		t.Errorf("dirs: %v", cfg.Dirs)
	}
	if got := cfg.Exclude["macbook"]; len(got) != 1 || got[0] != "vim" {
		t.Errorf("exclude.macbook: %v", got)
	}
}
