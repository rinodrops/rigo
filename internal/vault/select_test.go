package vault

import (
	"testing"
)

func selection_config(t *testing.T) map[string]Entry {
	t.Helper()
	return map[string]Entry{
		"vimrc":    {Path: ".vimrc"},
		"vim":      {Path: ".vim", Dir: true},
		"zshrc":    {Path: ".zshrc"},
		"ssh_conf": {Path: ".ssh/config"},
		"hs":       {Path: ".hammerspoon", Dir: true},
	}
}

const selection_toml = `
dirs = [".hammerspoon/"]
[tags]
vim = [".vim/", ".vimrc"]
[groups]
work = ["workpc", "buildbox"]
[include]
buildbox = ["zsh"]
[exclude]
work    = ["vim"]
macbook = [".hammerspoon/"]
mini    = [".ssh"]
[secrets]
`

func TestSelectExcludeMode(t *testing.T) {
	cfg := load_config(t, selection_toml)
	entries := selection_config(t)

	// workpc excludes the vim tag through its group.
	sel := Select(cfg, "workpc")
	if sel.Mode != ModeExclude {
		t.Fatalf("mode: %s", sel.Mode)
	}
	if sel.Selected(entries["vimrc"]) || sel.Selected(entries["vim"]) {
		t.Error("vim tag members should be excluded on workpc")
	}
	if !sel.Selected(entries["zshrc"]) || !sel.Selected(entries["hs"]) {
		t.Error("unrelated entries should stay selected on workpc")
	}

	// macbook excludes one path directly.
	sel = Select(cfg, "macbook")
	if sel.Selected(entries["hs"]) {
		t.Error(".hammerspoon should be excluded on macbook")
	}
	if !sel.Selected(entries["vimrc"]) {
		t.Error(".vimrc should be selected on macbook")
	}

	// mini excludes a container directory: files beneath it follow.
	sel = Select(cfg, "mini")
	if sel.Selected(entries["ssh_conf"]) {
		t.Error(".ssh/config should be excluded via the .ssh prefix")
	}

	// A host with no entries anywhere deploys everything.
	sel = Select(cfg, "fresh")
	for name, e := range entries {
		if !sel.Selected(e) {
			t.Errorf("%s should be selected on a fresh host", name)
		}
	}
}

func TestSelectIncludeMode(t *testing.T) {
	cfg := load_config(t, selection_toml)
	// buildbox has an include list, so it is whitelist-mode; it also
	// inherits work's exclude of the vim tag.
	cfg.Tags["zsh"] = []string{".zshrc"}
	entries := selection_config(t)

	sel := Select(cfg, "buildbox")
	if sel.Mode != ModeInclude {
		t.Fatalf("mode: %s", sel.Mode)
	}
	if !sel.Selected(entries["zshrc"]) {
		t.Error(".zshrc should be included on buildbox")
	}
	for _, name := range []string{"vimrc", "vim", "hs", "ssh_conf"} {
		if sel.Selected(entries[name]) {
			t.Errorf("%s should not be selected in include mode", name)
		}
	}
}
