package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write_config(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rigo.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFull(t *testing.T) {
	c, err := Load(write_config(t, `
dirs   = [".hammerspoon/"]
ignore = ["*.bak", "**/node_modules/"]

os_dir    = ".platform"
trash_dir = ".bin"

[tags]
vim    = [".vim/", ".vimrc", ".config/nvim/"]
claude = [".claude/settings.json"]

[groups]
work     = ["workpc", "buildbox"]
personal = ["macbook"]

[include]
servers = ["zsh", ".gitconfig"]

[exclude]
work    = ["vim", "claude"]
macbook = [".hammerspoon/"]

[secrets]
".config/gh/hosts.yml" = "op://Personal/GitHub CLI/hosts.yml"
".netrc"               = { ref = "op://Personal/netrc/notesPlain", mode = 0o640 }
".config/foo/token"    = { ref = "op://Work/foo/credential", os = ["darwin", "linux"] }
`))
	if err != nil {
		t.Fatal(err)
	}

	if c.OSDir != ".platform" || c.AbsDir != DefaultAbsDir || c.TrashDir != ".bin" {
		t.Errorf("special dirs: got %q %q %q", c.OSDir, c.AbsDir, c.TrashDir)
	}
	if len(c.Dirs) != 1 || c.Dirs[0] != ".hammerspoon/" {
		t.Errorf("dirs: got %v", c.Dirs)
	}
	if got := c.Tags["vim"]; len(got) != 3 {
		t.Errorf("tags.vim: got %v", got)
	}
	if got := c.Groups["work"]; len(got) != 2 || got[0] != "workpc" {
		t.Errorf("groups.work: got %v", got)
	}
	if got := c.Exclude["macbook"]; len(got) != 1 || got[0] != ".hammerspoon/" {
		t.Errorf("exclude.macbook: got %v", got)
	}

	hosts := c.Secrets[".config/gh/hosts.yml"]
	if hosts.Ref != "op://Personal/GitHub CLI/hosts.yml" || hosts.Mode != 0o600 {
		t.Errorf("string secret: got %+v", hosts)
	}
	netrc := c.Secrets[".netrc"]
	if netrc.Ref != "op://Personal/netrc/notesPlain" || netrc.Mode != 0o640 {
		t.Errorf("table secret: got %+v", netrc)
	}
	token := c.Secrets[".config/foo/token"]
	if len(token.OS) != 2 || token.OS[0] != "darwin" {
		t.Errorf("os-limited secret: got %+v", token)
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(write_config(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.OSDir != ".os" || c.AbsDir != ".abs" || c.TrashDir != ".trash" {
		t.Errorf("defaults: got %q %q %q", c.OSDir, c.AbsDir, c.TrashDir)
	}
	if c.Secrets != nil {
		t.Errorf("secrets: got %v", c.Secrets)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"unknown top-level key", `vault = "/somewhere"`, "unknown keys"},
		{"absolute dir path", `dirs = ["/etc/hosts"]`, "relative to the home"},
		{"dotdot path", `dirs = ["../outside"]`, ".."},
		{"empty tag path", "[tags]\nvim = [\"\"]", "empty path"},
		{"uppercase host", "[groups]\nwork = [\"WorkPC\"]", "lowercase"},
		{"dotted host", "[groups]\nwork = [\"pc.local\"]", "no dots"},
		{"uppercase include key", "[include]\nWork = [\"vim\"]", "lowercase"},
		{"group and host collision", "[groups]\na = [\"b\"]\nb = [\"c\"]", "both a group name and a host name"},
		{"secret without scheme", `[secrets]
".netrc" = "Personal/netrc"`, "backend scheme"},
		{"secret unknown key", `[secrets]
".netrc" = { ref = "op://x/y", when = "always" }`, "unknown key"},
		{"secret bad mode", `[secrets]
".netrc" = { ref = "op://x/y", mode = 4096 }`, "mode"},
		{"secret bad os", `[secrets]
".netrc" = { ref = "op://x/y", os = ["plan9"] }`, "os entries"},
		{"secret absolute path", `[secrets]
"/etc/token" = "op://x/y"`, "relative to the home"},
		{"os_dir with separator", `os_dir = "nested/os"`, "plain directory name"},
		{"uppercase distro", `distros = ["Ubuntu"]`, "lowercase os-release ID"},
		{"dot-prefixed distro", `distros = [".hidden"]`, "lowercase os-release ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write_config(t, tc.content))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}
