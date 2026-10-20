#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=$(awk '$1 == "go" { print $2; exit }' "$repo_dir/go.mod")
if [ "${1:-}" = "--modernize" ]; then
    # The modernize analyzer requires a newer SDK than Evergreen itself.
    version=1.26.0
    shift
fi

# GOOS and GOARCH describe the build target, not the machine running the SDK.
case "$(uname -s)" in
    Darwin) host_os=darwin ;;
    Linux) host_os=linux ;;
    CYGWIN*|MINGW*|MSYS*) host_os=windows ;;
    *) echo "Unsupported Go SDK host OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64) host_arch=amd64 ;;
    aarch64|arm64) host_arch=arm64 ;;
    ppc64le|s390x) host_arch=$(uname -m) ;;
    *) echo "Unsupported Go SDK host architecture: $(uname -m)" >&2; exit 1 ;;
esac

platform=$host_os-$host_arch
extension=tar.gz
go_executable=go
if [ "$host_os" = windows ]; then
    extension=zip
    go_executable=go.exe
fi
archive=go$version.$platform.$extension
checksum=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$repo_dir/scripts/go-sdk-checksums.txt")
if [ ${#checksum} -ne 64 ]; then
    echo "No pinned SHA-256 checksum for $archive in scripts/go-sdk-checksums.txt" >&2
    exit 1
fi

sdk_cache=$repo_dir/bin/go-sdk
sdk_dir=$sdk_cache/go$version.$platform
sdk_root=$sdk_dir/go
action=${1:-install}
if [ $# -gt 0 ]; then
    shift
fi
case "$action" in
    platform) echo "$host_os $host_arch"; exit 0 ;;
    path) echo "$sdk_root"; exit 0 ;;
    install|go|exec) ;;
    *) echo "Usage: $0 [--modernize] [install|path|platform|go ARGS...|exec COMMAND...]" >&2; exit 1 ;;
esac

sdk_installed() {
    [ -x "$sdk_root/bin/$go_executable" ] &&
        [ -f "$sdk_dir/.sha256" ] &&
        [ "$(< "$sdk_dir/.sha256")" = "$checksum" ]
}

install_sdk() (
    mkdir -p "$sdk_cache"
    # Serialize separate make processes as well as parallel targets. Publish only
    # a fully extracted SDK so interrupted downloads cannot poison the cache.
    attempts=0
    until mkdir "$sdk_dir.lock" 2>/dev/null; do
        if sdk_installed; then
            return
        fi
        attempts=$((attempts + 1))
        if [ "$attempts" -ge 180 ]; then
            echo "Timed out waiting for $sdk_dir.lock; remove it if no SDK download is running." >&2
            exit 1
        fi
        sleep 1
    done
    download_dir=
    trap 'if [ -n "$download_dir" ]; then rm -rf "$download_dir"; fi; rmdir "$sdk_dir.lock"' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    if sdk_installed; then
        return
    fi
    if [ -e "$sdk_dir" ]; then
        echo "Incomplete Go SDK at $sdk_dir; remove this directory and retry." >&2
        exit 1
    fi
    download_dir=$(mktemp -d "$sdk_cache/.download.XXXXXX")
    echo "Downloading $archive" >&2
    curl --fail --location --silent --show-error --retry 5 --retry-max-time 120 \
        --output "$download_dir/$archive" "https://go.dev/dl/$archive"
    (
        cd "$download_dir"
        if command -v sha256sum >/dev/null 2>&1; then
            printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check >&2
        else
            printf '%s  %s\n' "$checksum" "$archive" | shasum -a 256 --check >&2
        fi
        if [ "$extension" = zip ]; then
            unzip -q "$archive"
        else
            tar -xzf "$archive"
        fi
        test -x "go/bin/$go_executable"
        printf '%s\n' "$checksum" > .sha256
        rm "$archive"
    )
    mv "$download_dir" "$sdk_dir"
    download_dir=
)

if ! sdk_installed; then
    install_sdk
fi

export GOROOT="$sdk_root"
if [ "$host_os" = windows ]; then
    export GOROOT="$(cygpath -m "$sdk_root")"
fi
export PATH="$sdk_root/bin:$PATH"
# Ignore persistent user settings and disable Go's automatic toolchain switching.
export GOENV=off GOTOOLCHAIN=local
case "$action" in
    go) exec "$sdk_root/bin/$go_executable" "$@" ;;
    exec) exec "$@" ;;
esac
