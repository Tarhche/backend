#!/bin/sh
#
# Prints the vmhost image's fingerprint, which the image is tagged with.
#
# Redeploying a vmhost stops every VM on its node, so its image is rebuilt and
# redeployed only when what it is built from changes, and this is what says
# so. It is a hash of:
#
#   - the source of every package of this module the vmhost command depends
#     on, file by file, with the microsandbox tag (the engine) and for both
#     architectures the image is built for;
#   - the path and version of every other module those packages come from,
#     and go.mod's go and toolchain lines. That is what go.mod and go.sum say
#     about the vmhost, so a module it does not use can change without
#     restarting a VM;
#   - main.go, which registers the command;
#   - the Dockerfile, and what its vmhost stages copy: scripts/vmhost/, and
#     burn-pids;
#   - this script.
#
#   scripts/vmhost-fingerprint.sh              the fingerprint
#   scripts/vmhost-fingerprint.sh --manifest   what it is a hash of, to see
#                                              why it changed
#
# It needs go, and sha256sum or shasum.
set -eu

cd "$(dirname "$0")/.."
root=$(pwd)

command=./presentation/commands/workload/vmhost
architectures="amd64 arm64"

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$@"
	else
		shasum -a 256 "$@"
	fi
}

# packages lists what the vmhost command depends on, for every architecture,
# as format says.
packages() {
	for arch in $architectures; do
		CGO_ENABLED=1 GOOS=linux GOARCH=$arch go list -deps -tags microsandbox -f "$1" "$command"
	done
}

manifest() {
	# this module's own source, file by file
	packages '{{if and .Module .Module.Main}}{{$dir := .Dir}}{{range .GoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .CgoFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .CFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .HFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{range .EmbedFiles}}{{$dir}}/{{.}}{{"\n"}}{{end}}{{end}}' |
		sed "s|^$root/||" | sort -u | while read -r file; do
			sha256 "$file"
		done

	# every other module it takes a package from, by version
	packages '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{with .Replace}} => {{.Path}} {{.Version}}{{end}}{{end}}{{end}}' |
		sed '/^$/d' | sort -u

	grep -E '^(go|toolchain) ' go.mod

	{
		printf '%s\n' main.go Dockerfile scripts/vmhost-fingerprint.sh
		find scripts -type f \( -path 'scripts/vmhost/*' -o -name '*burn-pids*' \)
	} | sort -u | while read -r file; do
		sha256 "$file"
	done
}

if [ "${1:-}" = "--manifest" ]; then
	manifest
	exit 0
fi

manifest | sha256 | cut -c1-16
