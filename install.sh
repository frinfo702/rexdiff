#!/bin/sh
# install.sh installs the latest rexdiff release for this Mac:
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

[ "$(uname -s)" = Darwin ] || fail "rexdiff runs on macOS only"

case $(uname -m) in
arm64) arch=arm64 ;;
x86_64) arch=amd64 ;;
*) fail "unsupported machine: $(uname -m)" ;;
esac

name="rexdiff-darwin-$arch.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

curl -fsSL "$base/$name" -o "$tmp/$name" ||
	fail "cannot download $base/$name"
curl -fsSL "$base/sha256sums.txt" -o "$tmp/sha256sums.txt" ||
	fail "cannot download $base/sha256sums.txt"

want=$(awk -v name="$name" '$2 == name { print $1 }' "$tmp/sha256sums.txt")
[ -n "$want" ] || fail "the checksums have no $name"
got=$(shasum -a 256 "$tmp/$name" | awk '{ print $1 }')
[ "$want" = "$got" ] || fail "$name does not match its checksum"

tar -xzf "$tmp/$name" -C "$tmp" rexdiff || fail "cannot unpack $name"
mkdir -p "$dir"
install -m 0755 "$tmp/rexdiff" "$dir/.rexdiff-new"
mv -f "$dir/.rexdiff-new" "$dir/rexdiff"
printf 'installed %s\n' "$dir/rexdiff"

case ":$PATH:" in
*":$dir:"*) ;;
*)
	printf 'add %s to PATH, for example in ~/.zshrc:\n' "$dir"
	printf '  export PATH="%s:$PATH"\n' "$dir"
	;;
esac
