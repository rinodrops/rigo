# Rigo

<p align="center">
  English | <a href="README.ja.md">日本語</a>
</p>

Dotfiles manager for macOS, Linux, and Windows.

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
| `rigo add <path>`                           | Move real content into the vault and link it back (`--os`, `--dir`/`--files`, `--tag`, `--keep-symlink`, `--volume`) |
| `rigo forget <path>`                        | Stop managing: materialize locally, move the vault copy to the trash                                                 |
| `rigo diff [<path>]`                        | Show differences between local files and the vault (read-only; exit 1 when they differ)                              |
| `rigo clean`                                | Clean up broken links, offering restores from the trash                                                              |
| `rigo tag link/unlink/show <name>`          | Bulk operations on tagged groups                                                                                     |
| `rigo trash ls/restore/empty`               | Inspect, restore, or purge trashed vault entries                                                                     |
| `rigo secrets apply/status/remove [<path>]` | Materialize secrets from a password manager (1Password `op://` refs)                                                 |
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
# automatically). Anything not named here is a container: only the
# files inside are managed individually.
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
`.os/<goos>/.abs/`. The host a machine identifies as is its hostname up
to the first dot, lowercased (`rigo status` shows it).

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
