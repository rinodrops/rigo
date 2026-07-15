set windows-shell := ["sh", "-cu"]

exe_name := "rigo"
version  := `awk -F'"' '/^const Version =/{print $2; exit}' version.go`
dist     := "dist"

# --- top-level ---------------------------------------------------------

default: help

help:
    just --list

test:
    go test ./...

clean:
    rm -rf {{dist}}

[macos]
dev: darwin-build-arm64

[linux]
dev: linux-build-x86_64

[windows]
dev: win-build-x86_64

release: darwin-release linux-release win-release

# Print the README release-history section for a tag (Release body)
release-notes tag:
    @awk '/^### {{tag}}([^0-9.]|$)/{found=1; next} \
        (/^### v/ || /^## /) && found {exit} found' README.md

# --- generic cross-compile (internal) ----------------------------------

_build goos goarch outdir ext="":
    mkdir -p "{{dist}}/{{outdir}}"
    CGO_ENABLED=0 GOOS={{goos}} GOARCH={{goarch}} \
        go build -trimpath -ldflags="-s -w" \
        -o "{{dist}}/{{outdir}}/{{exe_name}}{{ext}}" .

_zip outdir arch_label os_label ext="":
    cd "{{dist}}/{{outdir}}" && zip -X \
        "../{{exe_name}}-v{{version}}-{{os_label}}-{{arch_label}}.zip" \
        "{{exe_name}}{{ext}}"
    @echo "zip created: {{dist}}/{{exe_name}}-v{{version}}-{{os_label}}-{{arch_label}}.zip"

# --- macOS --------------------------------------------------------------

darwin-build: darwin-build-arm64 darwin-build-x86_64

darwin-build-arm64: (_build "darwin" "arm64" "darwin-arm64")

darwin-build-x86_64: (_build "darwin" "amd64" "darwin-x86_64")

darwin-sign-arm64: darwin-build-arm64 (_darwin-sign "darwin-arm64")

darwin-sign-x86_64: darwin-build-x86_64 (_darwin-sign "darwin-x86_64")

# Hardened-runtime signing of the bare CLI binary (no .app bundle)
_darwin-sign outdir:
    codesign --force --options runtime --timestamp \
        --sign "$APPLE_DEVELOPER_CERTIFICATE_NAME" \
        "{{dist}}/{{outdir}}/{{exe_name}}"

darwin-notarize-arm64: darwin-sign-arm64 (_zip "darwin-arm64" "arm64" "darwin") (_darwin-notarize "arm64")

darwin-notarize-x86_64: darwin-sign-x86_64 (_zip "darwin-x86_64" "x86_64" "darwin") (_darwin-notarize "x86_64")

# Notarize the release zip. Flat Mach-O binaries cannot be stapled
# (stapling requires an .app/.dmg/.pkg); Gatekeeper fetches the ticket
# online, which is sufficient for a CLI tool.
_darwin-notarize arch:
    xcrun notarytool submit \
        "{{dist}}/{{exe_name}}-v{{version}}-darwin-{{arch}}.zip" \
        --apple-id "$APPLE_ID" \
        --team-id "$APPLE_DEVELOPER_TEAM_ID" \
        --password "$APPLE_DEVELOPER_APP_PASSWORD" \
        --wait

darwin-release: darwin-notarize-arm64 darwin-notarize-x86_64

# --- Linux --------------------------------------------------------------

linux-build: linux-build-x86_64 linux-build-arm64

linux-build-x86_64: (_build "linux" "amd64" "linux-x86_64")

linux-build-arm64: (_build "linux" "arm64" "linux-arm64")

linux-zip-x86_64: linux-build-x86_64 (_zip "linux-x86_64" "x86_64" "linux")

linux-zip-arm64: linux-build-arm64 (_zip "linux-arm64" "arm64" "linux")

# .deb/.rpm via nfpm. The concrete config is generated from the
# nfpm.yaml template with sed (@VAR@ placeholders); nfpm's own env var
# expansion does not cover the contents src path.
_linux-pkg arch nfpm_arch:
    sed -e "s/@VERSION@/{{version}}/" \
        -e "s/@RIGO_ARCH@/{{arch}}/" \
        -e "s/@NFPM_ARCH@/{{nfpm_arch}}/" \
        nfpm.yaml > "{{dist}}/nfpm-{{arch}}.yaml"
    nfpm package -f "{{dist}}/nfpm-{{arch}}.yaml" -p deb -t {{dist}}
    nfpm package -f "{{dist}}/nfpm-{{arch}}.yaml" -p rpm -t {{dist}}
    rm "{{dist}}/nfpm-{{arch}}.yaml"

linux-pkg-x86_64: linux-build-x86_64 (_linux-pkg "x86_64" "amd64")

linux-pkg-arm64: linux-build-arm64 (_linux-pkg "arm64" "arm64")

linux-release: linux-zip-x86_64 linux-zip-arm64 linux-pkg-x86_64 linux-pkg-arm64

# --- Windows ------------------------------------------------------------

win-build: win-build-x86_64 win-build-arm64

win-build-x86_64: (_build "windows" "amd64" "windows-x86_64" ".exe")

win-build-arm64: (_build "windows" "arm64" "windows-arm64" ".exe")

win-zip-x86_64: win-build-x86_64 (_zip "windows-x86_64" "x86_64" "windows" ".exe")

win-zip-arm64: win-build-arm64 (_zip "windows-arm64" "arm64" "windows" ".exe")

win-release: win-zip-x86_64 win-zip-arm64

# --- install ------------------------------------------------------------

[macos]
install: darwin-build-arm64
    mkdir -p "$HOME/.local/bin"
    cp "{{dist}}/darwin-arm64/{{exe_name}}" "$HOME/.local/bin/{{exe_name}}"
    chmod +x "$HOME/.local/bin/{{exe_name}}"

[linux]
install: linux-build-x86_64
    mkdir -p "$HOME/.local/bin"
    cp "{{dist}}/linux-x86_64/{{exe_name}}" "$HOME/.local/bin/{{exe_name}}"
    chmod +x "$HOME/.local/bin/{{exe_name}}"

[windows]
install: win-build-x86_64
    mkdir -p "$LOCALAPPDATA/Programs/{{exe_name}}"
    cp "{{dist}}/windows-x86_64/{{exe_name}}.exe" \
        "$LOCALAPPDATA/Programs/{{exe_name}}/"
