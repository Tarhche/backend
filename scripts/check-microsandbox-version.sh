#!/bin/sh
# Holds microsandbox to one release everywhere.
#
# The workload-microsandbox service links microsandbox's SDK, at the version
# go.mod requires, and runs the msb that comes in the image it is built on, at
# the version the Dockerfile names. The two share one database under MSB_HOME,
# and an SDK and an msb of different versions break it. So this fails when
# go.mod and the Dockerfile name different releases, when the Dockerfile's
# image is not pinned by its digest, or when go.mod points the SDK somewhere a
# version does not describe.
#
# It reads files and nothing else, so it runs anywhere, CI or not:
#
#   scripts/check-microsandbox-version.sh
set -eu

cd "$(dirname "$0")/.."

module=github.com/superradcompany/microsandbox/sdk/go

fail() {
	echo "check-microsandbox-version: $*" >&2
	exit 1
}

# the requirement, whether go.mod writes it in a require block or on its own
sdk=$(awk -v module="$module" '
	$1 == "require" && $2 == module { print $3; exit }
	$1 == module { print $2; exit }
' go.mod)
[ -n "$sdk" ] || fail "go.mod does not require $module"

if grep -Eq "^[[:space:]]*(replace[[:space:]]+)?$module([[:space:]]+v[^[:space:]]+)?[[:space:]]+=>" go.mod; then
	fail "go.mod replaces $module, so its version no longer says which SDK is built"
fi

arg() {
	sed -n "s/^ARG $1=//p" Dockerfile | head -n 1
}

image=$(arg MICROSANDBOX_VERSION)
digest=$(arg MICROSANDBOX_DIGEST)

[ -n "$image" ] || fail "the Dockerfile sets no MICROSANDBOX_VERSION"

if [ "${sdk#v}" != "$image" ]; then
	fail "go.mod requires the SDK at $sdk, but the Dockerfile builds on microsandbox $image; bump both together"
fi

echo "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$' ||
	fail "the Dockerfile's MICROSANDBOX_DIGEST is not a digest: '$digest'"

echo "microsandbox $image: the SDK in go.mod and the image the Dockerfile builds on agree ($digest)"
