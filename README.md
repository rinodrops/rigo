# Rigo

<p align="center">
  English | <a href="README.ja.md">日本語</a>
</p>

Dotfiles manager for macOS, Linux, and Windows.

Full documentation, including a step-by-step tutorial and command and
configuration references, is available at
<https://emotiongraphics.jp/docs/ref/rigo>.

Rigo keeps the real files in a **vault** — a plain directory you sync
across your machines with whatever tool you like (Syncthing, Dropbox,
iCloud Drive, ...) — and symlinks them into place. Rigo itself never
talks to the sync mechanism. Editing a linked file edits the vault copy
directly, so changes propagate with no extra "apply" step. There is no
manifest and no templating: the vault's directory tree itself is the
single source of truth, mirroring your home directory
(`vault/.zshrc` → `~/.zshrc`).

<p align="center">
  <a href="https://github.com/rinodrops/rigo/releases/latest">
    <img src="https://img.shields.io/github/v/release/rinodrops/rigo?color=orange&label=Download" alt="Latest Release">
  </a>
  <img src="https://img.shields.io/badge/macOS-13%2B-blue" alt="macOS 13+">
  <img src="https://img.shields.io/badge/Linux-blue" alt="Linux">
  <img src="https://img.shields.io/badge/Windows-11-blue" alt="Windows 11">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8" alt="Built with Go">
</p>

## Install

Download a release archive and put `rigo` on your `PATH`, or build from
source:

```sh
go install github.com/rinodrops/rigo@latest
```

Linux `.deb` / `.rpm` packages are also attached to releases.

On Windows, Rigo creates symlinks, which requires either Developer Mode
(Settings → System → For developers) or an elevated prompt.

## Quick start

```sh
# First run on a new machine: point rigo at the config inside your vault
rigo -f ~/Sync/Vault/.config/rigo/rigo.toml apply

# From then on, the vault is discovered automatically
rigo status

# Start managing a file (moves it into the vault, links it back)
rigo add ~/.zshrc

# Manage a whole directory as one symlink, grouped under a tag
rigo add --dir --tag vim ~/.vim

# Stop managing something (the local file stays; the vault copy is trashed)
rigo forget ~/.zshrc
```

## Commands

| Command                                     | Purpose                                                                                                              |
| ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `rigo apply`                                | Converge: link everything pending/unlinked; list conflicts                                                           |
| `rigo status [<path>]`                      | Show the state of managed entries                                                                                    |
| `rigo link <path>`                          | Link one entry (interactive on conflict; `--force` prefers the vault)                                                |
| `rigo unlink <path>`                        | Materialize locally for a while (the vault copy is kept)                                                             |
| `rigo add <path>`                           | Move real content into the vault and link it back (`--os`, `--flavour`, `--dir`/`--files`, `--tag`, `--keep-symlink`, `--volume`) |
| `rigo forget <path>`                        | Stop managing: materialize locally, move the vault copy to the trash                                                 |
| `rigo diff [<path>]`                        | Show differences between local files and the vault (read-only; exit 1 when they differ)                              |
| `rigo clean`                                | Clean up broken links, offering restores from the trash                                                              |
| `rigo tag link/unlink/show <name>`          | Bulk operations on tagged groups                                                                                     |
| `rigo trash ls/restore/empty`               | Inspect, restore, or purge trashed vault entries                                                                     |
| `rigo secrets apply/status/remove [<path>]` | Materialize secrets from a password manager (1Password `op://` refs)                                                 |
| `rigo version`                              | Print the version and os/arch (also `--version` / `-v`)                                                              |
| `rigo -f <path> <command>`                  | First-run bootstrap: name the `rigo.toml` inside the vault directly                                                  |

Entry states: `linked`, `pending`, `unlinked`, `conflict`, `broken`
(plus `excluded` in status output). Conflicts are never resolved
silently — Rigo shows a diff and asks, or you pass `--force`.

## Configuration

A single TOML file lives *inside the vault* at
`<vault>/.config/rigo/rigo.toml` and is linked to
`~/.config/rigo/rigo.toml` like any other managed file. It annotates
the vault — it is not a manifest. Everything below is optional:

```toml
# Directories deployed as ONE symlink (new files inside follow
# automatically). For any directory not named here, only the files
# inside are managed individually.
dirs = [".hammerspoon/"]

# Extra ignore patterns (gitignore-style globs against vault paths).
# OS droppings and sync-service artifacts are ignored built-in.
ignore = ["*.bak", "**/node_modules/"]

# Which directories under .os/linux/ are distro overlays,
# matched against the ID field of /etc/os-release.
distros = ["ubuntu", "debian"]

[tags]                       # name → members; naming a directory here
vim = [".vim/", ".vimrc"]    # also declares directory-unit deployment

[groups]                     # group → hosts (Ansible inventory style)
work = ["workpc", "buildbox"]

[include]                    # host/group → ONLY these deploy (allowlist)
servers = ["zsh", ".gitconfig"]

[exclude]                    # host/group → these do NOT deploy
work = ["vim"]

[extra]                      # host/group → deploy ONLY on these hosts
pis = ["gpio", ".config/pi.toml"]  # other hosts see them as excluded

# Optional renames for vault structural directories (defaults shown):
# os_dir = ".os"
# abs_dir = ".abs"
# trash_dir = ".trash"
# flavour_dir = ".flavour"

[volumes]                    # named volumes for Windows drives:
data = { default = "d", workpc = "e" }
# vault/.os/windows/.abs/data/Tools/x.ini → D:\Tools\x.ini (E:\ on workpc).
# The built-in volume "system" is %SystemDrive% and needs no declaration.

[secrets]                    # target path → password-manager reference
".netrc"           = { ref = "op://Personal/netrc/notesPlain", mode = 0o600 }
".config/gh/token" = "op://Personal/GitHub/token"
```

OS-specific entries live under `vault/.os/<darwin|linux|windows>/`,
mirroring home the same way. Absolute paths outside home go under
`.os/<goos>/.abs/`. Linux distro overlays live under
`.os/linux/<id>/` when listed in `distros`.

**OS flavours** cover environment classes that share a GOOS (and often
a distro ID) but need different files — today the only built-in is
`wsl`. Place them under `.os/<goos>/.flavour/<name>/` (for example
`.os/linux/.flavour/wsl/`). Rigo detects WSL via
`/proc/sys/fs/binfmt_misc/WSLInterop`, `/run/WSL`, or `microsoft` in
`/proc/sys/kernel/osrelease`. Layer order (later wins on the same
logical path): common → OS → distro → flavour. Use
`rigo add --flavour wsl <path>` to store under the flavour layer;
`--os` alone still means `.os/<goos>/` and does not auto-route to a
flavour. `rigo status` shows the detected flavour when present.

**Selective deployment** (`[include]` / `[exclude]` / `[extra]`) is
host/group oriented:

- `[include]` is an allowlist — once a host has any include entries,
  only those paths/tags deploy. Poor fit when most files are shared.
- `[exclude]` is a denylist — good for “almost everything, omit a few.”
- `[extra]` is an affirmative bundle — paths/tags listed there deploy
  only on the hosts/groups that name them; everywhere else they show as
  `excluded`. Entries never listed under `[extra]` keep the usual
  include/exclude rules. Use this when most of the vault is shared and
  a few files belong to a host group (for example a Pi fleet). It is
  independent of include mode: an `[extra]` target deploys on that host
  even if the host is in allowlist mode and the path is not in
  `[include]`. `[exclude]` still wins when both apply.
- For environment-class differences (WSL vs bare metal, etc.), prefer
  flavour (or distro) overlays in the vault tree, not include/exclude/
  extra.

The host a machine identifies as is its hostname up to the first dot,
lowercased (`rigo status` shows it).

## Development

Requires Go and [just](https://github.com/casey/just).

```sh
just test     # run tests
just dev      # build for the host platform into dist/
just help     # list all recipes
```

The 1Password integration test is opt-in: `RIGO_TEST_OP=1 go test
./internal/secrets/` (it needs the `op` CLI and a dedicated test item).

Releases are built by CI from version tags; the release notes are taken
from the matching section below (`just release-notes v1.0.0`).

## Release history

### v1.2.0 — 2026-07-23

Add `[extra]` for affirmative selective deployment: list paths or tags
under a host or group so those entries deploy only there, while the rest
of the vault keeps the usual `[include]` / `[exclude]` rules. Other hosts
see them as `excluded`. `[exclude]` still wins when both apply, and
`[extra]` stays independent of allowlist mode. Document the section in
both READMEs.

### v1.1.0 — 2026-07-22

Add OS flavour overlays for environment classes that share a GOOS (and
often a distro ID) but need different files. The first built-in flavour
is `wsl`: place entries under `.os/linux/.flavour/wsl/`, detect WSL at
runtime, and store with `rigo add --flavour wsl`. Layer order (later
wins) is common → OS → distro → flavour. Document when to use
`[include]` / `[exclude]` versus flavours. Also stream file comparisons
for status/apply, probe Windows symlink capability once before linking,
and expand unit tests around volumes and CLI helpers.

### v1.0.4 — 2026-07-20

Add a `version` subcommand following the common Go CLI convention.
All three invocations (`rigo version`, `--version`, `-v`) print the
same line with the os/arch appended, for example
`rigo version 1.0.4 darwin/arm64`.

### v1.0.3 — 2026-07-19

Reword user-facing text: the directory-add prompt and the `--files` help
no longer use the term "container"; per-file handling is described
directly. Link the full documentation site (tutorials, command and
configuration references) from both READMEs.

### v1.0.2 — 2026-07-18

Expand a leading `~` in the path arguments of `add` and the global
`-f` flag, matching the other commands. Shells that pass `~` through
to external commands literally (notably PowerShell) can now use the
quick-start examples as written.

### v1.0.1 — 2026-07-16

Fix path arguments for `status`, `link`, `unlink`, `forget`, and `diff`
so absolute and home paths (for example `~/.zshrc`) resolve to the same
vault entry as the logical path.

### v1.0.0 — 2026-07-15

Initial release. Vault + symlink dotfiles management for macOS, Linux,
and Windows: `status` / `apply` / `link` / `unlink` / `add` / `forget` /
`diff` / `tag` / `clean` / `trash` / `secrets` (1Password), per-machine
selection via groups/include/exclude, and named volumes for Windows
drives.

## License

MIT — see [LICENSE](LICENSE).
