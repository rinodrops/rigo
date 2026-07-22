package vault

import (
	"testing"

	"github.com/rinodrops/rigo/internal/config"
)

func TestResolveVolumes(t *testing.T) {
	cfg := load_config(t, `
[groups]
work = ["workpc", "buildbox"]
lab  = ["buildbox"]

[volumes]
data    = { default = "d", work = "e" }
scratch = "s"
sysalt  = { workpc = "q" }
`)
	host := func(name string) Host { return Host{Name: name, GOOS: "windows", SysDrive: "C:"} }

	got, err := resolve_volumes(cfg, host("workpc"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"system": "c", "data": "e", "scratch": "s", "sysalt": "q"} {
		if got[name] != want {
			t.Errorf("workpc %s: got %q, want %q", name, got[name], want)
		}
	}

	got, err = resolve_volumes(cfg, host("other"))
	if err != nil {
		t.Fatal(err)
	}
	if got["data"] != "d" {
		t.Errorf("other data: got %q, want default d", got["data"])
	}
	if _, ok := got["sysalt"]; ok {
		t.Error("sysalt should be unresolved on other")
	}

	// Conflicting letters from two groups of the same host.
	cfg.Volumes["data"] = config.Volume{Hosts: map[string]string{"work": "e", "lab": "f"}}
	if _, err := resolve_volumes(cfg, host("buildbox")); err == nil {
		t.Error("expected a group-conflict error")
	}
}

func TestSystemLetter(t *testing.T) {
	if got := system_letter(Host{SysDrive: "E:"}); got != "e" {
		t.Errorf("got %q", got)
	}
	if got := system_letter(Host{}); got != "c" {
		t.Errorf("empty SysDrive default: %q", got)
	}
}
