#!/usr/bin/env bash
# Copyright 2025 Emiliano Spinella (eminwux)
# SPDX-License-Identifier: Apache-2.0
#
# install.sh — one-line installer for sbsh.
#
#   curl -fsSL https://raw.githubusercontent.com/eminwux/sbsh/main/scripts/install.sh | bash
#
# Collapses the multi-step manual install (download → chmod → install →
# hardlink) documented in README §Install into a single invocation. sbsh is a
# self-contained binary with no host prerequisites (no daemon, no containerd,
# no cgroups), so — unlike the kukeon installer this is adapted from — there
# are no prereq checks, no `init` step, and no systemd unit.
#
# `sb` is created as a real hardlink (not a symlink) because the binary
# dispatches on argv[0] basename: `sb` and `sbsh` are the same inode, and the
# subcommand surface is selected by the name it is invoked under.
#
# Env overrides:
#   SBSH_VERSION           pin a release tag, e.g. v0.12.5 (default: resolve latest via GitHub API)
#   SBSH_REPO              GitHub repo path (default: eminwux/sbsh — for forks)
#   SBSH_INSTALL_PREFIX    install dir (default: /usr/local/bin)
#   SBSH_SKIP_CHECKSUM=1   skip SHA256 verification (NOT recommended)
#
# A GITHUB_TOKEN (or GH_TOKEN) in the environment is sent as a bearer token on
# the GitHub API call used to resolve the latest tag — this avoids the low
# unauthenticated rate limit on shared/CI runners. It is optional.

set -euo pipefail

SBSH_REPO="${SBSH_REPO:-eminwux/sbsh}"
SBSH_INSTALL_PREFIX="${SBSH_INSTALL_PREFIX:-/usr/local/bin}"
SBSH_VERSION="${SBSH_VERSION:-}"
SBSH_SKIP_CHECKSUM="${SBSH_SKIP_CHECKSUM:-}"

# --- Output helpers -----------------------------------------------------------
# Colors are emitted only on a TTY so piped output (e.g. CI logs) stays clean.
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_RESET=$'\033[0m'
    C_BOLD=$'\033[1m'
    C_GREEN=$'\033[32m'
    C_RED=$'\033[31m'
    C_YELLOW=$'\033[33m'
    C_BLUE=$'\033[34m'
else
    C_RESET=""; C_BOLD=""; C_GREEN=""; C_RED=""; C_YELLOW=""; C_BLUE=""
fi

ok()   { printf '%s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
fail() { printf '%s✗%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; }
step() { printf '\n%s==>%s %s\n' "$C_BOLD$C_BLUE" "$C_RESET" "$*"; }

# --- Platform detection ------------------------------------------------------
# Maps `uname` output to the published asset name `sbsh-<os>-<arch>`. sbsh
# publishes linux/darwin/freebsd × amd64/arm64 (see Makefile OS/ARCHS and
# .github/workflows/release.yaml).
detect_os() {
    local kernel
    kernel="$(uname -s)"
    case "$kernel" in
        Linux)   echo "linux" ;;
        Darwin)  echo "darwin" ;;
        FreeBSD) echo "freebsd" ;;
        *)
            fail "unsupported OS: ${kernel} (supported: Linux, Darwin, FreeBSD)"
            exit 1
            ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            fail "unsupported architecture: ${arch} (supported: amd64, arm64)"
            exit 1
            ;;
    esac
}

# --- Privilege helper --------------------------------------------------------
# Use sudo only when the install prefix is not already writable by the current
# user — root installs need no escalation, and a user-writable prefix (e.g. a
# custom SBSH_INSTALL_PREFIX, or CI) installs without prompting at all. Falls
# back to sudo for the usual /usr/local/bin-owned-by-root case.
SUDO=""
maybe_sudo() {
    local dir="$1"
    # Walk up to the nearest existing ancestor: the prefix may not exist yet,
    # in which case writability of the parent that will hold it is what matters.
    while [ -n "$dir" ] && [ ! -e "$dir" ]; do
        dir="$(dirname "$dir")"
    done
    if [ -w "$dir" ]; then
        SUDO=""
        return
    fi
    if [ "$(id -u)" -eq 0 ]; then
        SUDO=""
        return
    fi
    if ! command -v sudo >/dev/null 2>&1; then
        fail "install prefix ${SBSH_INSTALL_PREFIX} is not writable and \`sudo\` is not installed."
        fail "Re-run as root, install sudo, or set SBSH_INSTALL_PREFIX to a writable directory."
        exit 1
    fi
    SUDO="sudo"
}

# --- Version resolution ------------------------------------------------------
resolve_latest_version() {
    # Resolve the literal latest release tag via the GitHub API rather than the
    # "/releases/latest" redirect alias — the alias is mutable and can silently
    # roll back to a withdrawn release. The API call returns the exact tag_name
    # we then bake into the download URL.
    local api_url="https://api.github.com/repos/${SBSH_REPO}/releases/latest"
    local -a auth=()
    local token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
    if [ -n "$token" ]; then
        auth=(-H "Authorization: Bearer ${token}")
    fi
    local resp
    # ${auth[@]+...} guards the empty-array expansion: under `set -u`, a bare
    # "${auth[@]}" on an empty array aborts with "unbound variable" on bash
    # <4.4 (macOS stock /bin/bash 3.2.57) — the default no-token one-liner path.
    if ! resp="$(curl -fsSL ${auth[@]+"${auth[@]}"} "$api_url" 2>/dev/null)"; then
        fail "could not query ${api_url} for the latest release tag."
        printf '    Pin a version manually:  SBSH_VERSION=v0.12.5 bash install.sh\n' >&2
        exit 1
    fi
    # Stay grep/sed-only so the script has no jq dependency.
    local tag
    tag="$(printf '%s\n' "$resp" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
    if [ -z "$tag" ]; then
        fail "could not parse tag_name from GitHub API response."
        exit 1
    fi
    printf '%s\n' "$tag"
}

# --- Install -----------------------------------------------------------------
# Global so the EXIT trap below can see it after do_install returns. A `local`
# binding would be invisible by the time the trap fires in the outer shell.
INSTALL_TMPDIR=""
cleanup_tmpdir() {
    if [ -n "${INSTALL_TMPDIR}" ] && [ -d "${INSTALL_TMPDIR}" ]; then
        rm -rf "${INSTALL_TMPDIR}"
    fi
}

verify_checksum() {
    local bin_path="$1" sha_url="$2" sha_path="$3"

    if [ -n "$SBSH_SKIP_CHECKSUM" ]; then
        warn "SBSH_SKIP_CHECKSUM=1 set — skipping checksum verification (not recommended)."
        return 0
    fi

    if ! curl -fsSL -o "$sha_path" "$sha_url" 2>/dev/null; then
        # No checksum asset is published yet (depends on #56). Degrade
        # gracefully with a loud warning rather than hard-failing the install
        # — but never silently: the operator is told verification was skipped
        # and how to suppress the warning.
        warn "no checksum asset published at ${sha_url} — proceeding WITHOUT verification."
        warn "Set SBSH_SKIP_CHECKSUM=1 to acknowledge and silence this warning."
        return 0
    fi

    # Checksum assets are published in `sha256sum` format ("<hex>  <name>").
    local expected actual
    expected="$(awk '{print $1}' "$sha_path")"
    if [ -z "$expected" ]; then
        fail "checksum asset at ${sha_url} is empty or malformed."
        exit 1
    fi
    actual="$(sha256sum "$bin_path" | awk '{print $1}')"
    if [ "$expected" != "$actual" ]; then
        fail "checksum mismatch for the downloaded asset"
        printf '    expected: %s\n' "$expected" >&2
        printf '    actual:   %s\n' "$actual" >&2
        exit 1
    fi
    ok "sha256 ${actual}"
}

do_install() {
    local os arch asset_url sha_url bin_path sha_path
    os="$(detect_os)"
    arch="$(detect_arch)"

    step "Resolving release version"
    if [ -z "$SBSH_VERSION" ]; then
        SBSH_VERSION="$(resolve_latest_version)"
    fi
    ok "version: ${SBSH_VERSION}"

    asset_url="https://github.com/${SBSH_REPO}/releases/download/${SBSH_VERSION}/sbsh-${os}-${arch}"
    sha_url="${asset_url}.sha256"

    INSTALL_TMPDIR="$(mktemp -d -t sbsh-install.XXXXXX)"
    trap cleanup_tmpdir EXIT
    bin_path="${INSTALL_TMPDIR}/sbsh"
    sha_path="${INSTALL_TMPDIR}/sbsh.sha256"

    step "Downloading sbsh ${SBSH_VERSION} (${os}/${arch})"
    if ! curl -fsSL -o "$bin_path" "$asset_url"; then
        fail "download failed: ${asset_url}"
        printf '    Confirm SBSH_VERSION=%s ships a sbsh-%s-%s asset at\n' "$SBSH_VERSION" "$os" "$arch" >&2
        printf '      https://github.com/%s/releases\n' "$SBSH_REPO" >&2
        exit 1
    fi
    ok "downloaded $(wc -c <"$bin_path" | tr -d ' ') bytes"

    step "Verifying checksum"
    verify_checksum "$bin_path" "$sha_url" "$sha_path"

    chmod +x "$bin_path"

    step "Installing to ${SBSH_INSTALL_PREFIX}"
    maybe_sudo "$SBSH_INSTALL_PREFIX"
    $SUDO install -d -m 0755 "$SBSH_INSTALL_PREFIX"
    $SUDO install -m 0755 "$bin_path" "${SBSH_INSTALL_PREFIX}/sbsh"
    # Hardlink, not symlink — the binary dispatches on argv[0] basename, so
    # `sb` must resolve to the same inode rather than via a `sb -> sbsh`
    # indirection that some shells flatten.
    $SUDO ln -f "${SBSH_INSTALL_PREFIX}/sbsh" "${SBSH_INSTALL_PREFIX}/sb"
    ok "installed sbsh + sb hardlink at ${SBSH_INSTALL_PREFIX}"
}

# --- Next steps --------------------------------------------------------------
print_next_steps() {
    cat <<EOF

${C_GREEN}${C_BOLD}✓ sbsh installed${C_RESET}

Make sure ${SBSH_INSTALL_PREFIX} is on your PATH, then verify:
  ${C_BOLD}sbsh version${C_RESET}
  ${C_BOLD}sb get terminals${C_RESET}

Enable shell autocomplete (bash shown; zsh/fish also supported):
  ${C_BOLD}cat >> ~/.bashrc <<'RC'
source <(sbsh autocomplete bash)
source <(sb autocomplete bash)
RC${C_RESET}

Docs:    https://sbsh.io
Issues:  https://github.com/${SBSH_REPO}/issues
EOF
}

# --- Main --------------------------------------------------------------------
do_install
print_next_steps
