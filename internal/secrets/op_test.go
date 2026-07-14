package secrets

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestOpIntegration exercises the real 1Password backend against the
// dedicated test item (Personal vault, item "rigo-test"). It never
// runs in CI: it needs the op CLI, an unlocked session, and
// RIGO_TEST_OP=1.
func TestOpIntegration(t *testing.T) {
	if os.Getenv("RIGO_TEST_OP") != "1" {
		t.Skip("set RIGO_TEST_OP=1 to run the op integration test")
	}
	if _, err := exec.LookPath("op"); err != nil {
		t.Skip("op CLI not found")
	}

	refs := map[string]string{
		"op://Personal/rigo-test/password":   "rigo-op-password-1",
		"op://Personal/rigo-test/token":      "rigo-op-token-1",
		"op://Personal/rigo-test/notesPlain": "rigo op note",
	}
	for ref, want := range refs {
		got, err := op_read(ref)
		if err != nil {
			t.Fatalf("%s: %v (is the rigo-test item present and the session unlocked?)", ref, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", ref, got, want)
		}
	}

	// Full apply cycle with the real backend.
	item := Item{
		Path:   ".config/rigo-test/token",
		Ref:    "op://Personal/rigo-test/token",
		Mode:   0o600,
		Target: filepath.Join(t.TempDir(), ".config", "rigo-test", "token"),
	}
	if err := Apply(nil, item); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(item.Target)
	if err != nil || string(data) != "rigo-op-token-1" {
		t.Fatalf("applied content: %q %v", data, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(item.Target); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode: %v", fi.Mode())
		}
	}
	if gone, err := Remove(item); err != nil || !gone {
		t.Errorf("remove: %v %v", gone, err)
	}
}
