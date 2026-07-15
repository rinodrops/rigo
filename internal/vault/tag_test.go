package vault

import (
	"strings"
	"testing"
)

func TestMembers(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	make_tree(t, root, map[string]string{
		".vim/vimrc": "v",
		".vimrc":     "r",
	})
	cfg := load_config(t, `
[tags]
vim   = [".vim/", ".vimrc", ".config/nvim/"]
empty = []
`)
	entries, _, err := Scan(root, cfg, darwin_host(home))
	if err != nil {
		t.Fatal(err)
	}

	members, missing, err := Members(cfg, entries, "vim")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].Path != ".vim" || members[1].Path != ".vimrc" {
		t.Errorf("members: %+v", members)
	}
	if !members[0].Dir {
		t.Error(".vim should be a directory-unit member")
	}
	if len(missing) != 1 || missing[0] != ".config/nvim/" {
		t.Errorf("missing: %v", missing)
	}

	if m, missing, err := Members(cfg, entries, "empty"); err != nil || len(m) != 0 || len(missing) != 0 {
		t.Errorf("empty tag: %v %v %v", m, missing, err)
	}

	_, _, err = Members(cfg, entries, "nope")
	if err == nil || !strings.Contains(err.Error(), "known tags: empty, vim") {
		t.Errorf("undefined tag error: %v", err)
	}
}
