package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rinodrops/rigo/internal/vault"
)

// fixture builds an entry with vault content and an optional local
// state prepared by prep.
func fixture(t *testing.T, prep func(e vault.Entry)) vault.Entry {
	t.Helper()
	e := vault.Entry{
		Path:   ".zshrc",
		Vault:  filepath.Join(t.TempDir(), ".zshrc"),
		Target: filepath.Join(t.TempDir(), ".zshrc"),
	}
	if err := os.WriteFile(e.Vault, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if prep != nil {
		prep(e)
	}
	return e
}

func TestDiffEntryStates(t *testing.T) {
	cases := []struct {
		name    string
		prep    func(e vault.Entry)
		differs bool
		want    string
	}{
		{"pending", nil, false, "not deployed"},
		{"linked", func(e vault.Entry) {
			if err := os.Symlink(e.Vault, e.Target); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
		}, false, "same file"},
		{"unlinked", func(e vault.Entry) {
			if err := os.WriteFile(e.Target, []byte("content\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, false, "identical"},
		{"broken", func(e vault.Entry) {
			if err := os.Symlink(filepath.Join(filepath.Dir(e.Vault), "gone"), e.Target); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
		}, false, "broken symlink"},
		{"conflict", func(e vault.Entry) {
			if err := os.WriteFile(e.Target, []byte("content\nextra\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, true, "+extra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := fixture(t, tc.prep)
			var out strings.Builder
			differs, err := diff_entry(&out, e)
			if err != nil {
				t.Fatal(err)
			}
			if differs != tc.differs {
				t.Errorf("differs = %v, want %v", differs, tc.differs)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("output %q does not mention %q", out.String(), tc.want)
			}
		})
	}
}
