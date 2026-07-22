package vault

import (
	"os"
	"strings"
)

// Built-in OS flavour names. Unknown directories under .flavour/ are
// warned about and skipped (they are not home content).
const FlavourWSL = "wsl"

// KnownFlavour reports whether name is a built-in OS flavour.
func KnownFlavour(name string) bool {
	return name == FlavourWSL
}

// detect_linux_flavour returns the flavour for the running Linux host,
// or "" when none matches. At most one flavour is returned.
func detect_linux_flavour() string {
	return detect_wsl(
		"/proc/sys/fs/binfmt_misc/WSLInterop",
		"/run/WSL",
		"/proc/sys/kernel/osrelease",
	)
}

func detect_wsl(interop, run_wsl, osrelease string) string {
	if file_exists(interop) || file_exists(run_wsl) {
		return FlavourWSL
	}
	if data, err := os.ReadFile(osrelease); err == nil {
		if strings.Contains(strings.ToLower(string(data)), "microsoft") {
			return FlavourWSL
		}
	}
	return ""
}

func file_exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
