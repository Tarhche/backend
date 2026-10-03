#!/usr/bin/env bash
#
# Prints the fingerprint of vmhost's own code, which the backend workflow tags
# workload-vmhost's image with. vmhost is deployed by that tag, so it is rebuilt
# and redeployed when its code changes and only then: a deploy of the rest of the
# backend leaves the running vmhost alone, and with it every terminal and every
# connection to a microVM it is carrying, which would end with it. Its microVMs
# survive a redeploy either way; those connections do not.
#
# vmhost's own code is:
#
#   - its packages, and the guest agent every microVM boots, with every package
#     of this module they import and the version of every other module those
#     come from, for both architectures the image is built for. Tests are not
#     compiled into vmhost, and are not in it;
#   - the files of its own in packages it shares with the other services: its
#     serve command, its wiring, its configuration, and the main package that
#     registers it. The composition root they import (infrastructure/ioc/
#     providers) wires every service, the blog's included, so it is left out:
#     with it, nearly every change to the backend would redeploy vmhost;
#   - the Go release go.mod names, and the Dockerfile, which pins firecracker,
#     the kernel and the base images. Changing the Dockerfile is the way to
#     rebuild vmhost on purpose, for a newer Alpine for instance.
#
# A change to shared code alone (telemetry, the console, the composition root)
# reaches vmhost with its next change of its own.
#
# Run it from anywhere in the repository; --manifest prints what it hashed.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

packages=(
	./application/workload/vmhost
	./presentation/http/workload/vmhost
	./infrastructure/workload/vmm
	./infrastructure/repository/vmstate
	./domain/workload/vm
	./domain/workload/guest
	./cmd/workload-guest
)
files=(
	presentation/commands/workload/vmhost/*.go
	infrastructure/ioc/providers/workload/vmhost.go
	infrastructure/configs/vmhost.go
	main.go
	main_linux.go
	Dockerfile
)
composition_root='/infrastructure/ioc/providers'

# the module's own packages by their files, any other module by its version
# shellcheck disable=SC2016 # a Go template, which go list expands, not the shell
template='{{if not .Standard}}{{if .Module}}{{if .Module.Main}}{{$dir := .Dir}}{{range .GoFiles}}file {{$dir}}/{{.}}{{"\n"}}{{end}}{{range .EmbedFiles}}file {{$dir}}/{{.}}{{"\n"}}{{end}}{{else}}module {{.Module.Path}} {{.Module.Version}}{{with .Module.Replace}} => {{.Path}} {{.Version}}{{end}}{{"\n"}}{{end}}{{end}}{{end}}'

patterns=()
for package in "${packages[@]}"; do
	if [[ -d $package ]]; then
		patterns+=("$package/...")
	fi
done
if ((${#patterns[@]} == 0)); then
	echo "vmhost-fingerprint: none of vmhost's packages is in this checkout" >&2
	exit 1
fi

listing() {
	local arch file
	for arch in amd64 arm64; do
		GOOS=linux GOARCH=$arch CGO_ENABLED=0 go list -deps -f "$template" "${patterns[@]}"
	done | grep -v "^file $PWD$composition_root/" || true
	for file in "${files[@]}"; do
		if [[ -f $file && $file != *_test.go ]]; then
			echo "file $PWD/$file"
		fi
	done
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum | cut -d' ' -f1
	else
		shasum -a 256 | cut -d' ' -f1
	fi
}

manifest() {
	local kind rest
	listing | sort -u | while read -r kind rest; do
		case $kind in
		file) printf 'file %s %s\n' "${rest#"$PWD"/}" "$(sha256 <"$rest")" ;;
		module) printf 'module %s\n' "$rest" ;;
		esac
	done
	grep -E '^(go|toolchain) ' go.mod
}

if [[ ${1:-} == --manifest ]]; then
	manifest
	exit 0
fi

fingerprint=$(manifest | sha256)
echo "${fingerprint:0:24}"
