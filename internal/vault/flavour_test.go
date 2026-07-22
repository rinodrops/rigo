package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnownFlavour(t *testing.T) {
	if !KnownFlavour(FlavourWSL) {
		t.Fatal("wsl should be known")
	}
	if KnownFlavour("container") || KnownFlavour("") {
		t.Fatal("unknown flavours must be rejected")
	}
}

func TestDetectWSL(t *testing.T) {
	dir := t.TempDir()
	interop := filepath.Join(dir, "WSLInterop")
	run := filepath.Join(dir, "runWSL")
	osrelease := filepath.Join(dir, "osrelease")

	if got := detect_wsl(interop, run, osrelease); got != "" {
		t.Fatalf("empty probes: got %q", got)
	}

	if err := os.WriteFile(osrelease, []byte("6.6.0-microsoft-standard-WSL2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detect_wsl(interop, run, osrelease); got != FlavourWSL {
		t.Fatalf("osrelease: got %q", got)
	}

	if err := os.Remove(osrelease); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(run, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := detect_wsl(interop, run, osrelease); got != FlavourWSL {
		t.Fatalf("run/WSL: got %q", got)
	}

	if err := os.Remove(run); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(interop, []byte("enabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detect_wsl(interop, run, osrelease); got != FlavourWSL {
		t.Fatalf("WSLInterop: got %q", got)
	}
}
