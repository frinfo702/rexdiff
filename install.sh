#!/bin/sh
# install.sh installs the latest rexdiff release for this Mac or Linux:
#
#   curl -fsSL https://raw.githubusercontent.com/frinfo702/rexdiff/main/install.sh | sh
#
# It puts the binary in ~/.local/bin, or in REXDIFF_BIN_DIR, and checks the
# checksum of the archive.

set -eu

repo=frinfo702/rexdiff
base="https://github.com/$repo/releases/latest/download"
dir=${REXDIFF_BIN_DIR:-"$HOME/.local/bin"}

fail() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported system: $(uname -s)" ;;
esac

case $(uname -m) in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) fail "unsupported machine: $(uname -m)" ;;
esac

name="rexdiff-$os-$arch.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

curl -fsSL "$base/$name" -o "$tmp/$name" ||
	fail "cannot download $base/$name"
curl -fsSL "$base/sha256sums.txt" -o "$tmp/sha256sums.txt" ||
	fail "cannot download $base/sha256sums.txt"

if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$name")
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$name")
else
	fail "need sha256sum or shasum to check the checksum"
fi
got=${got%% *}

want=$(awk -v name="$name" '$2 == name { print $1 }' "$tmp/sha256sums.txt")
[ -n "$want" ] || fail "the checksums have no $name"
[ "$want" = "$got" ] || fail "$name does not match its checksum"

tar -xzf "$tmp/$name" -C "$tmp" rexdiff || fail "cannot unpack $name"
mkdir -p "$dir"
install -m 0755 "$tmp/rexdiff" "$dir/.rexdiff-new"
mv -f "$dir/.rexdiff-new" "$dir/rexdiff"
printf 'installed %s\n' "$dir/rexdiff"

case ":$PATH:" in
*":$dir:"*) ;;
*)
	printf 'add %s to PATH, for example:\n' "$dir"
	printf '  export PATH="%s:$PATH"\n' "$dir"
	;;
esac
