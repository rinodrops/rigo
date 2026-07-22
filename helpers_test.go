package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

func TestParseAge(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30d", 30 * 24 * time.Hour, false},
		{"0d", 0, false},
		{"72h", 72 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"-1d", 0, true},
		{"-5h", 0, true},
		{"xd", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		got, err := parse_age(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: got %v (%v), want %v", tc.in, got, err, tc.want)
		}
	}
}

func TestPromptChoice(t *testing.T) {
	d := vault.Diff{Stat: "+1 -0", Unified: "+line\n", Lines: 1}
	in := bufio.NewScanner(strings.NewReader("1\n"))
	var out bytes.Buffer
	got, err := prompt_choice(in, &out, d)
	if err != nil || got != 1 {
		t.Fatalf("choice 1: got %d err %v", got, err)
	}

	// Lines above diff_auto_show enable the "d) show diff" option.
	big := strings.Repeat("+line\n", diff_auto_show+5)
	d = vault.Diff{Stat: "+n", Unified: big, Lines: strings.Count(big, "\n")}
	in = bufio.NewScanner(strings.NewReader("d\n2\n"))
	out.Reset()
	got, err = prompt_choice(in, &out, d)
	if err != nil || got != 2 {
		t.Fatalf("choice d then 2: got %d err %v\nout=%s", got, err, out.String())
	}
	if !strings.Contains(out.String(), "+line") {
		t.Errorf("diff not shown: %s", out.String())
	}

	in = bufio.NewScanner(strings.NewReader("q\n"))
	got, err = prompt_choice(in, &out, d)
	if err != nil || got != 3 {
		t.Fatalf("quit: got %d err %v", got, err)
	}
}

func TestCleanOneSkip(t *testing.T) {
	st := vault.Stale{Logical: ".x", Target: "/tmp/x", Dest: "/vault/x"}
	in := bufio.NewScanner(strings.NewReader("s\n"))
	var out bytes.Buffer
	if err := clean_one(in, &out, st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "d) delete") {
		t.Errorf("prompt missing: %s", out.String())
	}
}

func TestCheckAddable(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	cfg_path := filepath.Join(root, ".config", "rigo", "rigo.toml")
	if err := os.MkdirAll(filepath.Dir(cfg_path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg_path, []byte("ignore = [\"*.bak\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfg_path)
	if err != nil {
		t.Fatal(err)
	}
	host := vault.Host{Name: "mac", GOOS: "darwin", Home: home}
	entries, _, err := vault.Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{root: root, cfg: cfg, cfg_path: cfg_path, host: host, entries: entries}

	route := vault.Route{Logical: ".zshrc", VaultRel: ".zshrc"}
	if err := check_addable(s, route, false); err != nil {
		t.Fatalf("fresh path: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".zshrc"), []byte("z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _, err = vault.Scan(root, cfg, host)
	if err != nil {
		t.Fatal(err)
	}
	s.entries = entries
	if err := check_addable(s, route, false); err == nil || !strings.Contains(err.Error(), "already managed") {
		t.Fatalf("want already managed, got %v", err)
	}

	bak := vault.Route{Logical: "x.bak", VaultRel: "x.bak"}
	s.entries = nil
	if err := check_addable(s, bak, false); err == nil || !strings.Contains(err.Error(), "ignore") {
		t.Fatalf("want ignore error, got %v", err)
	}
}
