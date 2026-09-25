#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
trap 'echo "FAIL at line $LINENO" >&2' ERR
mkdir -p "$test_dir/tools" "$test_dir/archive/go/bin"

# Exercise extraction and environment isolation without a network or a Go installation.
cat > "$test_dir/archive/go/bin/go" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$GOROOT" "$GOTOOLCHAIN" "$GOENV"
command -v go
EOF
cat > "$test_dir/tools/uname" <<'EOF'
#!/usr/bin/env bash
case "$1" in
    -s) echo "$SDK_TEST_OS" ;;
    -m) echo "$SDK_TEST_ARCH" ;;
esac
EOF
cat > "$test_dir/tools/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo download >> "$SDK_TEST_DOWNLOADS"
if [ "${SDK_TEST_FAIL:-}" = true ]; then exit 22; fi
if [ "${SDK_TEST_SLOW:-}" = true ]; then sleep 1; fi
while [ "$1" != --output ]; do shift; done
cp "$SDK_TEST_ARCHIVE" "$2"
EOF
cat > "$test_dir/tools/go" <<'EOF'
#!/usr/bin/env bash
echo 'System Go must not be used' >&2
exit 99
EOF
chmod +x "$test_dir/archive/go/bin/go" "$test_dir/tools/"*
tar -czf "$test_dir/sdk.tar.gz" -C "$test_dir/archive" go
if command -v sha256sum >/dev/null 2>&1; then
    checksum=$(sha256sum "$test_dir/sdk.tar.gz" | awk '{print $1}')
else
    checksum=$(shasum -a 256 "$test_dir/sdk.tar.gz" | awk '{print $1}')
fi
export PATH="$test_dir/tools:$PATH"
export SDK_TEST_OS=Linux SDK_TEST_ARCH=x86_64
export SDK_TEST_ARCHIVE="$test_dir/sdk.tar.gz"
export SDK_TEST_DOWNLOADS="$test_dir/downloads"

fixture() {
    fixture_dir=$test_dir/$1
    mkdir -p "$fixture_dir/scripts"
    cp "$repo_dir/scripts/go-sdk.sh" "$fixture_dir/scripts/"
    cp "$repo_dir/scripts/go-sdk-checksums.txt" "$fixture_dir/scripts/"
    cp "$repo_dir/go.mod" "$fixture_dir/"
}

fixture platforms
while read -r os arch expected; do
    actual=$(SDK_TEST_OS="$os" SDK_TEST_ARCH="$arch" GOOS=windows GOARCH=386 \
        bash "$fixture_dir/scripts/go-sdk.sh" platform)
    test "$actual" = "$expected"
done <<'EOF'
Linux x86_64 linux amd64
Linux aarch64 linux arm64
Linux ppc64le linux ppc64le
Linux s390x linux s390x
Darwin x86_64 darwin amd64
Darwin arm64 darwin arm64
CYGWIN_NT-10.0 x86_64 windows amd64
MINGW64_NT-10.0 arm64 windows arm64
EOF
test ! -e "$SDK_TEST_DOWNLOADS"
echo 'PASS HostPlatformIgnoresCrossCompilationTarget'

if SDK_TEST_OS=Unsupported bash "$fixture_dir/scripts/go-sdk.sh" path 2>"$test_dir/error"; then exit 1; fi
grep -q 'Unsupported Go SDK host OS' "$test_dir/error"
printf 'module example.com/test\n\ngo 1.99.0\n' > "$fixture_dir/go.mod"
if bash "$fixture_dir/scripts/go-sdk.sh" install 2>"$test_dir/error"; then exit 1; fi
grep -q 'No pinned SHA-256 checksum' "$test_dir/error"
test ! -e "$SDK_TEST_DOWNLOADS"
echo 'PASS UnsupportedHostsAndUnpinnedVersionsFailBeforeDownload'

fixture 'install with spaces'
version=$(awk '$1 == "go" {print $2}' "$fixture_dir/go.mod")
printf '%s  go%s.linux-amd64.tar.gz\n' "$checksum" "$version" > "$fixture_dir/scripts/go-sdk-checksums.txt"
sdk_root=$(bash "$fixture_dir/scripts/go-sdk.sh" path)
actual=$(GOROOT=/invalid GOTOOLCHAIN=go1.99.0 GOENV=/invalid \
    bash "$fixture_dir/scripts/go-sdk.sh" go env)
expected=$(printf '%s\n' "$sdk_root" local off "$sdk_root/bin/go")
test "$actual" = "$expected"
SDK_TEST_FAIL=true bash "$fixture_dir/scripts/go-sdk.sh" install
test "$(wc -l < "$SDK_TEST_DOWNLOADS" | tr -d ' ')" = 1
actual=$(bash "$fixture_dir/scripts/go-sdk.sh" exec bash -c 'command -v go')
test "$actual" = "$sdk_root/bin/go"
echo 'PASS VerifiedSDKIgnoresSystemGoAndReusesOfflineCache'

fixture bad-checksum
sdk_root=$(bash "$fixture_dir/scripts/go-sdk.sh" path)
if bash "$fixture_dir/scripts/go-sdk.sh" install 2>"$test_dir/error"; then exit 1; fi
grep -qi 'checksum.*did NOT match\|FAILED' "$test_dir/error"
test ! -e "$sdk_root"
test ! -e "${sdk_root%/go}.lock"
test -z "$(ls -A "$fixture_dir/bin/go-sdk")"
echo 'PASS ChecksumMismatchLeavesNoSDKOrPartialDownload'

fixture retry
printf '%s  go%s.linux-amd64.tar.gz\n' "$checksum" "$version" > "$fixture_dir/scripts/go-sdk-checksums.txt"
if SDK_TEST_FAIL=true bash "$fixture_dir/scripts/go-sdk.sh" install 2>"$test_dir/error"; then exit 1; fi
test -z "$(ls -A "$fixture_dir/bin/go-sdk")"
bash "$fixture_dir/scripts/go-sdk.sh" install
echo 'PASS FailedDownloadCanBeRetried'

fixture concurrent
printf '%s  go%s.linux-amd64.tar.gz\n' "$checksum" "$version" > "$fixture_dir/scripts/go-sdk-checksums.txt"
export SDK_TEST_DOWNLOADS="$test_dir/concurrent-downloads"
SDK_TEST_SLOW=true bash "$fixture_dir/scripts/go-sdk.sh" install &
first_pid=$!
SDK_TEST_SLOW=true bash "$fixture_dir/scripts/go-sdk.sh" install &
second_pid=$!
wait "$first_pid"
wait "$second_pid"
test "$(wc -l < "$SDK_TEST_DOWNLOADS" | tr -d ' ')" = 1
echo 'PASS ConcurrentInstallsShareOneCompleteSDK'

# Check Make's exports too: older macOS Make versions handle export differently.
cat > "$test_dir/env.mk" <<'EOF'
sdk-test-env:
	@test "$$GOROOT" = "$(GOROOT)"
	@test "$$GOTOOLCHAIN" = local
	@test "$$GOENV" = off
	@test "$${PATH%%:*}" = "$(goSDKRoot)/bin"
	@test "$(goos)_$(goarch)" = windows_amd64
EOF
make -s -C "$repo_dir" -f makefile -f "$test_dir/env.mk" sdk-test-env \
    GOROOT=/invalid GOTOOLCHAIN=go1.99.0 GOENV=/invalid GOOS=windows GOARCH=amd64
echo 'PASS MakeExportsPinnedSDKAndPreservesBuildTarget'
