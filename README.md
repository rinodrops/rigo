# Rigo

Dotfiles manager for macOS, Linux, and Windows.

Rigo keeps the real files in a **vault** — a plain directory you sync
across your machines with whatever tool you like (Syncthing, Dropbox,
iCloud Drive, ...) — and symlinks them into place. Rigo itself never
talks to the sync mechanism. Editing a
linked file edits the vault copy directly, so changes propagate with no
extra "apply" step. There is no manifest and no templating: the vault's
directory tree itself is the single source of truth, mirroring your home
directory (`vault/.zshrc` → `~/.zshrc`).

> Status: under initial development. Commands below reflect the design
> and may not all be implemented yet.

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

# Manage an OS-specific file
rigo add --os ~/.hammerspoon
```

## Commands

| Command                                     | Purpose                                                              |
| ------------------------------------------- | -------------------------------------------------------------------- |
| `rigo apply`                                | Converge: link everything pending/unlinked; list conflicts            |
| `rigo status [<path>]`                      | Show state of managed entries                                         |
| `rigo link <path>`                          | Link one entry (interactive on conflict; `--force` prefers the vault) |
| `rigo unlink <path>`                        | Materialize locally for a while (vault copy is kept)                  |
| `rigo add <path>`                           | Move a real file/dir into the vault and link it                       |
| `rigo forget <path>`                        | Stop managing: materialize locally, move the vault copy to trash      |
| `rigo diff [<path>]`                        | Show differences between local files and the vault (read-only)        |
| `rigo clean`                                | Clean up broken links                                                 |
| `rigo tag link/unlink/show <name>`          | Bulk operations on tagged groups                                      |
| `rigo trash ls/restore/empty`               | Inspect, restore, or purge trashed vault entries                      |
| `rigo secrets apply/status/remove [<path>]` | Materialize secrets from a backend (e.g., `op://…`)                   |

Entry states: `linked`, `pending`, `unlinked`, `conflict`, `broken`
(plus `excluded` in status output). Conflicts are never resolved
silently — Rigo asks, or you pass `--force`.

## Configuration

A single TOML file lives *inside the vault* at
`<vault>/.config/rigo/rigo.toml` and is linked to
`~/.config/rigo/rigo.toml` like any other managed file. It annotates the
vault (directory-level link units, tags, per-machine include/exclude,
secrets) — it is not a manifest.

## Development

Requires Go and [just](https://github.com/casey/just).

```sh
just test     # run tests
just dev      # build for the host platform into dist/
just help     # list all recipes
```

Releases are built by CI from version tags (`v*`).
