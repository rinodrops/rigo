# Rigo

<p align="center">
  <a href="README.md">English</a> | 日本語
</p>

macOS・Linux・Windows対応のdotfiles管理ツール。

チュートリアルとコマンド・設定リファレンスをまとめた詳しいドキュメントが
<https://emotiongraphics.jp/docs/ja/ref/rigo> にあります。

Rigoはdotfilesの実体を**Vault**（Syncthing・Dropbox・iCloud Driveなど
お好みのツールでマシン間同期する，ただのディレクトリ）に置き，
実際の場所へsymlinkを張ります。Rigo自身は同期機構に一切関知しません。
リンクされたファイルを編集するとVault側の実体が直接編集されるため，
変更の反映に「apply」のような追加操作は不要です。マニフェストも
テンプレートもありません。Vaultのディレクトリ木そのものが唯一の
情報源で，ホームディレクトリをそのまま鏡写しにします
（`vault/.zshrc` → `~/.zshrc`）。

<p align="center">
  <a href="https://github.com/rinodrops/rigo/releases/latest">
	<img src="https://img.shields.io/github/v/release/rinodrops/rigo?color=orange&label=Download" alt="Latest Release">
  </a>
  <img src="https://img.shields.io/badge/macOS-13%2B-blue" alt="macOS 13+">
  <img src="https://img.shields.io/badge/Linux-blue" alt="Linux">
  <img src="https://img.shields.io/badge/Windows-11-blue" alt="Windows 11">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8" alt="Built with Go">
</p>

## インストール

リリースのアーカイブをダウンロードして `rigo` を `PATH` に置くか，
ソースからビルドします:

```sh
go install github.com/rinodrops/rigo@latest
```

Linux向けには `.deb` / `.rpm` パッケージもリリースに添付されています。

WindowsではRigoがsymlinkを作成するため，開発者モード
（設定 → システム → 開発者向け）の有効化，または管理者権限での
実行が必要です。

## クイックスタート

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

## コマンド

| コマンド                                    | 役割                                                                                                     |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `rigo apply`                                | 収束: pending/unlinkedをすべてlinkし，conflictを一覧表示                                                   |
| `rigo status [<path>]`                      | 管理対象エントリの状態を表示                                                                                |
| `rigo link <path>`                          | 1エントリをlink（conflict時は対話，`--force`でVault優先）                                                   |
| `rigo unlink <path>`                        | 一時的に実体化（Vault側は保持，管理は継続）                                                                 |
| `rigo add <path>`                           | 実体をVaultへ移動してlink（`--os`，`--dir`/`--files`，`--tag`，`--keep-symlink`，`--volume`）              |
| `rigo forget <path>`                        | 管理をやめる: ローカルに実体化し，Vault側はtrashへ移動                                                     |
| `rigo diff [<path>]`                        | 実体とVaultの差分表示（読み取り専用。差分があれば終了コード1）                                              |
| `rigo clean`                                | 壊れたリンクの掃除（trashからの復元を提示）                                                                |
| `rigo tag link/unlink/show <name>`          | tag単位の一括操作                                                                                          |
| `rigo trash ls/restore/empty`               | trashの一覧・Vaultへの復元・完全削除                                                                        |
| `rigo secrets apply/status/remove [<path>]` | パスワードマネージャーからの機微情報の書き出し（1Passwordの`op://`参照）                                     |
| `rigo version`                              | バージョンとos/archの表示（`--version`・`-v`も可）                                                          |
| `rigo -f <path> <command>`                  | 初回ブートストラップ: Vault内の`rigo.toml`を直接指定                                                        |

エントリの状態は `linked`・`pending`・`unlinked`・`conflict`・`broken`
の5つ（statusでは加えて `excluded` を表示）です。conflictが黙って
解決されることはありません。Rigoはdiffを表示して確認するか，
`--force` の明示を求めます。

## 設定

設定はVault内の `<vault>/.config/rigo/rigo.toml` にある単一のTOML
ファイルだけで，他の管理対象ファイルと同様に
`~/.config/rigo/rigo.toml` へリンクされます。これはVaultへの注釈で
あって，マニフェストではありません。以下すべて省略可能です:

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

[volumes]                    # named volumes for Windows drives:
data = { default = "d", workpc = "e" }
# vault/.os/windows/.abs/data/Tools/x.ini → D:\Tools\x.ini (E:\ on workpc).
# The built-in volume "system" is %SystemDrive% and needs no declaration.

[secrets]                    # target path → password-manager reference
".netrc"           = { ref = "op://Personal/netrc/notesPlain", mode = 0o600 }
".config/gh/token" = "op://Personal/GitHub/token"
```

OS固有のエントリは `vault/.os/<darwin|linux|windows>/` 配下に，
同じくホームの鏡写しとして置きます。ホーム外の絶対パスは
`.os/<goos>/.abs/` 配下です。マシンの識別子はホスト名の最初の
ドットまでを小文字化したもので，`rigo status` のヘッダで確認できます。

## 開発

Goと[just](https://github.com/casey/just)が必要です。

```sh
just test     # run tests
just dev      # build for the host platform into dist/
just help     # list all recipes
```

1Password統合テストはオプトインです: `RIGO_TEST_OP=1 go test
./internal/secrets/`（`op` CLIと専用テストアイテムが必要）。

リリースはバージョンタグからCIがビルドします。リリースノートは
英語版READMEの該当節から抽出されます（`just release-notes v1.0.0`）。

## リリース履歴

### v1.0.4 — 2026-07-20

Go製CLIの慣習に合わせて `version` サブコマンドを追加。
`rigo version`・`--version`・`-v` の3通りすべてが，os/archを後置した
同一の行（例: `rigo version 1.0.4 darwin/arm64`）を表示します。

### v1.0.3 — 2026-07-19

ユーザー向け文言の整理: ディレクトリ追加時のプロンプトと `--files` の
ヘルプから「container」の語を廃し，ファイル個別の扱いを直接説明する
表現に変更。また両READMEの冒頭からドキュメントサイト（チュートリアル・
コマンド／設定リファレンス）へリンクした。

### v1.0.2 — 2026-07-18

`add` とグローバル `-f` フラグのパス引数で先頭の `~` を展開するよう
修正し，他コマンドと挙動を統一。`~` を外部コマンドへそのまま渡す
シェル（PowerShellなど）でも，クイックスタートの例がそのまま
動作します。

### v1.0.1 — 2026-07-16

`status` / `link` / `unlink` / `forget` / `diff` のパス引数が，絶対パスや
ホームパス（例: `~/.zshrc`）でも論理パスと同じVaultエントリに解決されるよう
修正。

### v1.0.0 — 2026-07-15

初回リリース。Vault + symlink方式のdotfiles管理（macOS / Linux /
Windows）: `status` / `apply` / `link` / `unlink` / `add` / `forget` /
`diff` / `tag` / `clean` / `trash` / `secrets`（1Password対応），
groups/include/excludeによるマシンごとの選択的展開，Windowsドライブの
名前付きボリューム。

## ライセンス

MIT — [LICENSE](LICENSE)を参照。
