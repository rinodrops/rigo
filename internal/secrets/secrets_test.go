package secrets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

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

func fake_runner(value string) Runner {
	return func(ref string) ([]byte, error) { return []byte(value), nil }
}

func TestPlanFiltersAndSorts(t *testing.T) {
	cfg := load_config(t, `
[secrets]
".netrc"            = "op://Personal/netrc/notesPlain"
".config/gh/token"  = { ref = "op://Personal/gh/token", mode = 0o640 }
".config/mac/only"  = { ref = "op://Personal/mac/x", os = ["darwin"] }
".excluded/token"   = "op://Personal/ex/x"

[exclude]
box = [".excluded/token"]
`)
	host := vault.Host{Name: "box", GOOS: "linux", Home: "/home/rino"}
	sel := vault.Select(cfg, "box")

	items, err := Plan(cfg, host, sel, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Path != ".config/gh/token" || items[1].Path != ".netrc" {
		t.Fatalf("items: %+v", items)
	}
	if items[0].Mode != 0o640 || items[1].Mode != 0o600 {
		t.Errorf("modes: %o %o", items[0].Mode, items[1].Mode)
	}
	if items[1].Target != filepath.Join("/home/rino", ".netrc") {
		t.Errorf("target: %q", items[1].Target)
	}
}

func TestPlanExplicitPath(t *testing.T) {
	cfg := load_config(t, `
[secrets]
".netrc"          = "op://Personal/netrc/notesPlain"
".excluded/token" = "op://Personal/ex/x"

[exclude]
box = [".excluded/token"]
`)
	host := vault.Host{Name: "box", GOOS: "linux", Home: "/home/rino"}
	sel := vault.Select(cfg, "box")

	items, err := Plan(cfg, host, sel, nil, ".netrc")
	if err != nil || len(items) != 1 {
		t.Fatalf("explicit: %v %v", items, err)
	}
	if _, err := Plan(cfg, host, sel, nil, ".excluded/token"); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Errorf("want excluded refusal, got %v", err)
	}
	if _, err := Plan(cfg, host, sel, nil, ".unknown"); err == nil || !strings.Contains(err.Error(), "not a secrets entry") {
		t.Errorf("want unknown error, got %v", err)
	}
}

func TestPlanVaultCollision(t *testing.T) {
	cfg := load_config(t, `
[secrets]
".netrc" = "op://Personal/netrc/notesPlain"
`)
	host := vault.Host{Name: "box", GOOS: "linux", Home: "/home/rino"}
	entries := []vault.Entry{{Path: ".netrc"}}

	_, err := Plan(cfg, host, vault.Select(cfg, "box"), entries, "")
	if err == nil || !strings.Contains(err.Error(), "both a vault entry and a secret") {
		t.Errorf("want collision error, got %v", err)
	}
}

func TestApplyWritesAndOverwrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), "deep", "nested", "token")
	item := Item{Path: "token", Ref: "op://x/y", Mode: 0o600, Target: target}

	if err := Apply(fake_runner("first"), item); err != nil {
		t.Fatal(err)
	}
	if err := Apply(fake_runner("second"), item); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "second" {
		t.Fatalf("content: %q %v", data, err)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(target)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode: %v %v", fi.Mode(), err)
		}
		parent, err := os.Stat(filepath.Dir(target))
		if err != nil || parent.Mode().Perm() != 0o700 {
			t.Errorf("parent mode: %v %v", parent.Mode(), err)
		}
	}
}

func TestUnknownScheme(t *testing.T) {
	item := Item{Path: "x", Ref: "vaultwarden://a/b", Mode: 0o600, Target: filepath.Join(t.TempDir(), "x")}
	if err := Apply(nil, item); err == nil || !strings.Contains(err.Error(), `no backend for scheme "vaultwarden"`) {
		t.Errorf("want scheme error, got %v", err)
	}
}

func TestAppliedAndRemove(t *testing.T) {
	item := Item{Path: "x", Ref: "op://x/y", Mode: 0o600, Target: filepath.Join(t.TempDir(), "x")}
	if Applied(item) {
		t.Error("should be missing before apply")
	}
	if err := Apply(fake_runner("v"), item); err != nil {
		t.Fatal(err)
	}
	if !Applied(item) {
		t.Error("should be applied")
	}
	gone, err := Remove(item)
	if err != nil || !gone {
		t.Errorf("remove: %v %v", gone, err)
	}
	gone, err = Remove(item)
	if err != nil || gone {
		t.Errorf("second remove: %v %v", gone, err)
	}
}
