#!/usr/bin/env sh

# Cross-platform kLang builder.
#
# The script itself uses portable POSIX shell syntax and can run from macOS,
# Linux, WSL, MSYS2, or Git Bash. Go performs the actual cross-compilation.

set -eu

SCRIPT_DIR=$(CDPATH= cd -P -- "$(dirname -- "$0")" && pwd)
BUILD_DIR=${KLANG_BUILD_DIR:-"$SCRIPT_DIR/build"}
BUILD_MODE=host
TARGET=
RUN_TESTS=0
DEBUG_BUILD=0

SUPPORTED_TARGETS="
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
"

usage() {
	cat <<'EOF'
Build kLang native executables and its browser WebAssembly runtime.

Usage:
  ./build.sh [options]

Build selection:
  (no option)          Build for the current host
  --target OS/ARCH     Build one Go target, for example windows/amd64
  --all                Build the release matrix and the WASM runtime
  --wasm               Build only the browser WASM runtime

Options:
  --test               Run the complete Go test suite before building
  --debug              Keep debug symbols (release builds strip them)
  --out DIRECTORY      Write artifacts to DIRECTORY
  --list-targets       Print the supported release matrix
  -h, --help           Show this help

Environment:
  KLANG_BUILD_DIR      Default artifact directory
  KLANG_CGO_ENABLED    CGO setting used for native builds (default: 0)
  GOFLAGS              Additional flags understood by the Go toolchain

Artifacts:
  build/native/OS-ARCH/kLang[.exe]
  build/wasm/klang.wasm
  build/wasm/wasm_exec.js

The shell script requires a POSIX-compatible shell. On Windows, run it through
Git Bash, MSYS2, or WSL. The produced Windows executable does not require that
shell.
EOF
}

say() {
	printf '%s\n' "==> $*"
}

fail() {
	printf '%s\n' "build error: $*" >&2
	exit 1
}

require_value() {
	[ "$#" -ge 2 ] || fail "$1 requires a value"
	[ -n "$2" ] || fail "$1 requires a non-empty value"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target)
			require_value "$@"
			BUILD_MODE=target
			TARGET=$2
			shift 2
			;;
		--target=*)
			BUILD_MODE=target
			TARGET=${1#*=}
			[ -n "$TARGET" ] || fail "--target requires OS/ARCH"
			shift
			;;
		--all)
			BUILD_MODE=all
			shift
			;;
		--wasm)
			BUILD_MODE=wasm
			shift
			;;
		--test)
			RUN_TESTS=1
			shift
			;;
		--debug)
			DEBUG_BUILD=1
			shift
			;;
		--out)
			require_value "$@"
			BUILD_DIR=$2
			shift 2
			;;
		--out=*)
			BUILD_DIR=${1#*=}
			[ -n "$BUILD_DIR" ] || fail "--out requires a directory"
			shift
			;;
		--list-targets)
			printf '%s' "$SUPPORTED_TARGETS" | sed '/^[[:space:]]*$/d'
			exit 0
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			fail "unknown option '$1'; run ./build.sh --help"
			;;
	esac
done

command -v go >/dev/null 2>&1 || fail "Go is not installed or is not on PATH"

cd "$SCRIPT_DIR"
[ -f go.mod ] || fail "go.mod was not found in $SCRIPT_DIR"

case "$BUILD_DIR" in
	/*) ;;
	*) BUILD_DIR="$SCRIPT_DIR/$BUILD_DIR" ;;
esac

mkdir -p "$BUILD_DIR"

GO_BUILD_FLAGS=-trimpath
if [ "$DEBUG_BUILD" -eq 0 ]; then
	GO_LDFLAGS="-s -w"
else
	GO_LDFLAGS=
fi

target_is_supported() {
	printf '%s' "$SUPPORTED_TARGETS" | sed '/^[[:space:]]*$/d' | grep -F -x "$1" >/dev/null 2>&1
}

build_native() {
	target=$1
	target_os=${target%/*}
	target_arch=${target#*/}

	[ "$target_os" != "$target" ] && [ -n "$target_os" ] && [ -n "$target_arch" ] ||
		fail "invalid target '$target'; expected OS/ARCH"
	target_is_supported "$target" ||
		fail "unsupported release target '$target'; run ./build.sh --list-targets"

	executable=kLang
	if [ "$target_os" = windows ]; then
		executable=kLang.exe
	fi

	output_dir="$BUILD_DIR/native/$target_os-$target_arch"
	output_path="$output_dir/$executable"
	mkdir -p "$output_dir"

	say "building native $target -> $output_path"
	if [ -n "$GO_LDFLAGS" ]; then
		CGO_ENABLED=${KLANG_CGO_ENABLED:-0} GOOS=$target_os GOARCH=$target_arch \
			go build "$GO_BUILD_FLAGS" -ldflags="$GO_LDFLAGS" -o "$output_path" .
	else
		CGO_ENABLED=${KLANG_CGO_ENABLED:-0} GOOS=$target_os GOARCH=$target_arch \
			go build "$GO_BUILD_FLAGS" -o "$output_path" .
	fi
}

find_wasm_exec() {
	go_root=$(go env GOROOT)
	for candidate in \
		"$go_root/lib/wasm/wasm_exec.js" \
		"$go_root/misc/wasm/wasm_exec.js"
	do
		if [ -f "$candidate" ]; then
			printf '%s\n' "$candidate"
			return 0
		fi
	done
	return 1
}

build_wasm() {
	output_dir="$BUILD_DIR/wasm"
	output_path="$output_dir/klang.wasm"
	mkdir -p "$output_dir"

	say "building browser WASM runtime -> $output_path"
	GOOS=js GOARCH=wasm CGO_ENABLED=0 \
		go build "$GO_BUILD_FLAGS" -o "$output_path" ./cmd/klang-wasm

	wasm_exec=$(find_wasm_exec) ||
		fail "wasm_exec.js was not found under the active Go installation"
	cp "$wasm_exec" "$output_dir/wasm_exec.js"
}

if [ "$RUN_TESTS" -eq 1 ]; then
	say "running Go tests"
	go test ./...
fi

case "$BUILD_MODE" in
	host)
		build_native "$(go env GOOS)/$(go env GOARCH)"
		;;
	target)
		build_native "$TARGET"
		;;
	all)
		printf '%s' "$SUPPORTED_TARGETS" |
			sed '/^[[:space:]]*$/d' |
			while IFS= read -r target; do
				build_native "$target"
			done
		build_wasm
		;;
	wasm)
		build_wasm
		;;
	*)
		fail "internal error: unknown build mode '$BUILD_MODE'"
		;;
esac

say "build complete: $BUILD_DIR"
