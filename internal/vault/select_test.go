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
		"pi":       {Path: ".config/pi.toml"},
	}
}

const selection_toml = `
dirs = [".hammerspoon/"]
[tags]
vim = [".vim/", ".vimrc"]
pi  = [".config/pi.toml"]
[groups]
work = ["workpc", "buildbox"]
pis  = ["living-pi", "office-pi"]
[include]
buildbox = ["zsh"]
[exclude]
work    = ["vim"]
macbook = [".hammerspoon/"]
mini    = [".ssh"]
[extra]
pis = ["pi"]
laptop = [".ssh/config"]
mini = [".ssh/config"]
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

	// mini excludes a plain directory: files beneath it follow.
	sel = Select(cfg, "mini")
	if sel.Selected(entries["ssh_conf"]) {
		t.Error(".ssh/config should be excluded via the .ssh prefix")
	}

	// A host with no entries anywhere deploys everything except
	// [extra]-constrained paths (those stay on their target hosts).
	sel = Select(cfg, "fresh")
	for name, e := range entries {
		if name == "pi" || name == "ssh_conf" {
			if sel.Selected(e) {
				t.Errorf("%s is [extra]-constrained and should be excluded on fresh", name)
			}
			continue
		}
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
	for _, name := range []string{"vimrc", "vim", "hs", "ssh_conf", "pi"} {
		if sel.Selected(entries[name]) {
			t.Errorf("%s should not be selected in include mode", name)
		}
	}
}

func TestSelectExtra(t *testing.T) {
	cfg := load_config(t, selection_toml)
	entries := selection_config(t)

	// Group member gets the tag listed under [extra].
	sel := Select(cfg, "living-pi")
	if !sel.Selected(entries["pi"]) {
		t.Error("pi tag should deploy on living-pi via group pis")
	}
	if !sel.Selected(entries["vimrc"]) {
		t.Error("unconstrained entries still deploy on living-pi")
	}

	// Non-member does not get the extra tag.
	sel = Select(cfg, "macbook")
	if sel.Selected(entries["pi"]) {
		t.Error("pi tag should be excluded on macbook")
	}

	// Path item under a host key: only that host.
	sel = Select(cfg, "laptop")
	if !sel.Selected(entries["ssh_conf"]) {
		t.Error(".ssh/config should deploy on laptop via [extra]")
	}
	sel = Select(cfg, "living-pi")
	if sel.Selected(entries["ssh_conf"]) {
		t.Error(".ssh/config should stay excluded on living-pi")
	}
}

func TestSelectExtraExcludeWins(t *testing.T) {
	cfg := load_config(t, selection_toml)
	entries := selection_config(t)

	// mini lists .ssh/config in [extra] but also excludes .ssh.
	sel := Select(cfg, "mini")
	if sel.Selected(entries["ssh_conf"]) {
		t.Error("exclude should win when it covers an [extra] path")
	}
}

func TestSelectExtraIncludeMode(t *testing.T) {
	cfg := load_config(t, selection_toml)
	cfg.Tags["zsh"] = []string{".zshrc"}
	cfg.Include["living-pi"] = []string{"zsh"}
	entries := selection_config(t)

	sel := Select(cfg, "living-pi")
	if sel.Mode != ModeInclude {
		t.Fatalf("mode: %s", sel.Mode)
	}
	// Extra-constrained and targeted: deploys even though not in include.
	if !sel.Selected(entries["pi"]) {
		t.Error("[extra] target should deploy independently of include")
	}
	if !sel.Selected(entries["zshrc"]) {
		t.Error("include list still applies to unconstrained entries")
	}
	if sel.Selected(entries["vimrc"]) {
		t.Error("unconstrained entries outside include stay out")
	}
}
