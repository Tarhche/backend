FROM golang:1.27-alpine AS base
WORKDIR /opt/app
COPY . .
ENV CGO_ENABLED=0
RUN go mod download

FROM base AS build
WORKDIR /opt/dist
RUN cd /opt/app \
    && go build -v -o app . \
    && chmod +x app \
    && cp ./app /opt/dist

# the agent every microVM boots as its init. It is a binary of its own, kept
# apart from the application's: the kernel unpacks it into every machine's
# memory before anything runs there, so what it carries is what every machine
# pays for, and what only a debugger reads is left out. It is static, since
# there is nothing in a machine for it to link against (CGO_ENABLED=0 above).
FROM base AS build-guest
RUN go build -v -trimpath -ldflags="-s -w" -o /opt/guest/workload-guest ./cmd/workload-guest

# what vmhost starts microVMs with, for the platform the image is built for:
# firecracker, and the kernel machines boot (Firecracker's own CI build of 6.1,
# which PR #101 proved with this agent). Both are pinned by version and by
# checksum, so a release that was replaced, or a download that was tampered
# with, fails the build instead of reaching a host. The checksums are of the
# release archive, as the release publishes them, and of the kernel itself.
#
# Nothing here runs anything built for the target, so it runs on the builder's
# own platform rather than under emulation, and only picks what it downloads by
# the target's architecture.
#
# Two things a build behind an unusual network can change, neither of which
# reaches an image that runs anywhere: where the two files are fetched from (a
# mirror; the checksums hold it to the same bytes), and a CA bundle to trust on
# top of Alpine's while fetching them, for a network that inspects TLS. The
# bundle is a build secret, so it is never written into a layer:
#
#   docker build --secret id=ca-certificates,src=<bundle.pem> ...
FROM --platform=$BUILDPLATFORM alpine:3.24 AS firecracker
ARG TARGETARCH
ARG FIRECRACKER_RELEASES=https://github.com/firecracker-microvm/firecracker/releases/download
ARG FIRECRACKER_VERSION=v1.17.0
ARG FIRECRACKER_SHA256_X86_64=06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558
ARG FIRECRACKER_SHA256_AARCH64=e351ebe4f7a16b5873bbd51005d2e6767103cff4d5ebc829df2d3f95a93e2256
ARG KERNEL_CI=https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci
ARG KERNEL_CI_VERSION=v1.15
ARG KERNEL_VERSION=6.1.155
ARG KERNEL_SHA256_X86_64=e20e46d0c36c55c0d1014eb20576171b3f3d922260d9f792017aeff53af3d4f2
ARG KERNEL_SHA256_AARCH64=e3544b10603acbf3db492cb52e000d22ba202cb4b63b9add027565683e11c591
RUN --mount=type=secret,id=ca-certificates \
    if [ -s /run/secrets/ca-certificates ]; then \
        cat /run/secrets/ca-certificates >> /etc/ssl/certs/ca-certificates.crt; \
    fi \
    && apk add --no-cache curl tar \
    && case "$TARGETARCH" in \
        amd64) arch=x86_64 firecracker_sha256="$FIRECRACKER_SHA256_X86_64" kernel_sha256="$KERNEL_SHA256_X86_64" ;; \
        arm64) arch=aarch64 firecracker_sha256="$FIRECRACKER_SHA256_AARCH64" kernel_sha256="$KERNEL_SHA256_AARCH64" ;; \
        *) echo "there is no firecracker for $TARGETARCH" >&2 && exit 1 ;; \
    esac \
    && curl -fsSLo /tmp/firecracker.tgz "${FIRECRACKER_RELEASES}/${FIRECRACKER_VERSION}/firecracker-${FIRECRACKER_VERSION}-${arch}.tgz" \
    && echo "${firecracker_sha256}  /tmp/firecracker.tgz" | sha256sum -c - \
    && tar -xzf /tmp/firecracker.tgz -C /tmp \
    && mkdir -p /opt/workload-vmhost/bin \
    && cp "/tmp/release-${FIRECRACKER_VERSION}-${arch}/firecracker-${FIRECRACKER_VERSION}-${arch}" /opt/workload-vmhost/bin/firecracker \
    && chmod 0755 /opt/workload-vmhost/bin/firecracker \
    && curl -fsSLo /opt/workload-vmhost/vmlinux "${KERNEL_CI}/${KERNEL_CI_VERSION}/${arch}/vmlinux-${KERNEL_VERSION}" \
    && echo "${kernel_sha256}  /opt/workload-vmhost/vmlinux" | sha256sum -c - \
    && chmod 0644 /opt/workload-vmhost/vmlinux \
    && rm -rf /tmp/firecracker.tgz "/tmp/release-${FIRECRACKER_VERSION}-${arch}"

# the same two files and nothing else, for taking out of a build rather than
# running: `make microvm-artifacts` exports them for the tests that boot real
# machines and for the Lima VM, so what those boot is exactly what vmhost's
# image carries, pinned in this one place.
FROM scratch AS microvm-artifacts
COPY --from=firecracker /opt/workload-vmhost/ /

FROM base AS develop
WORKDIR /opt/app
ENV PATH=$GOPATH/bin/linux_$GOARCH:$PATH
RUN apk add tmux
ENTRYPOINT ["go", "tool", "air", \
    "-build.poll=true", \
    "-build.poll_interval=2000", \
    "-build.include_ext=go,tmpl", \
    "-build.exclude_dir=tmp,vendor,testdata,.git,.github,resources/docs,storage", \
    "-build.exclude_regex=_test\\.go$", \
    "-build.stop_on_error=false", \
    "--"]

FROM alpine:latest AS production
RUN addgroup -g 10001 app \
    && adduser -u 10000 -g app -S -h /home/app app
USER app:app
COPY --chown=app:app --from=build /opt/dist /usr/bin
ENV GODEBUG=gctrace=1
ENTRYPOINT [ "app" ]

# blog service
FROM develop AS develop-blog
EXPOSE 80
CMD ["serve-blog", "--port=80"]

FROM production AS production-blog
EXPOSE 80
CMD ["serve-blog", "--port=80"]

# workload control plane service
FROM develop AS develop-workload-controlplane
EXPOSE 80
CMD ["serve-workload-controlplane", "--port=80"]

FROM production AS production-workload-controlplane
EXPOSE 80
CMD ["serve-workload-controlplane", "--port=80"]

# workload ingress service
FROM develop AS develop-workload-ingress
EXPOSE 80
CMD ["serve-workload-ingress", "--port=80"]

FROM production AS production-workload-ingress
EXPOSE 80
CMD ["serve-workload-ingress", "--port=80"]

# workload orchestrator service
FROM develop AS develop-workload-orchestrator
ENV WORKLOAD_ORCHESTRATOR_NAME=workload-orchestrator-01
EXPOSE 80
CMD ["serve-workload-orchestrator", "--port=80"]

FROM production AS production-workload-orchestrator
ENV WORKLOAD_ORCHESTRATOR_NAME=workload-orchestrator-01
EXPOSE 80
CMD ["serve-workload-orchestrator", "--port=80"]

# workload vmhost service: the daemon that runs microVMs for the orchestrator on
# its host. Unlike every other service it runs as root, because it makes
# machines' networks and firewall, starts their VMMs as users of their own and
# asks the host's systemd for their units. It makes images into the disks
# machines boot (sqfstar, which squashfs-tools has had since 4.6) and their
# scratch disks (mke2fs), writes its firewall whole (iptables-save and
# iptables-restore), pulls images over TLS (ca-certificates), and is asked
# whether it is healthy over its socket (curl); the build fails if any of the
# tools it runs is missing. It carries firecracker, the kernel and the agent
# machines boot, at the paths its configuration names by default, and copies
# them into its data directory at start, from where the host runs them.
#
# It is built on a pinned Alpine rather than the latest: its firewall rules are
# made in a network namespace that outlives every vmhost, so the iptables a new
# one runs must be the backend the last one wrote them with (nf_tables), and a
# base image that moves on by itself could change that between two deploys.
#
# In development the agent is built into the image as well: what air reloads
# is vmhost alone, so a change to the agent takes the image built again.
FROM develop AS develop-workload-vmhost
RUN apk add --no-cache ca-certificates curl e2fsprogs iptables squashfs-tools \
    && command -v sqfstar mke2fs iptables-save iptables-restore
COPY --from=build-guest /opt/guest/workload-guest /usr/bin/workload-guest
COPY --from=firecracker /opt/workload-vmhost/bin/firecracker /usr/local/bin/firecracker
COPY --from=firecracker /opt/workload-vmhost/vmlinux /opt/workload-vmhost/vmlinux
CMD ["serve-workload-vmhost"]

FROM alpine:3.24 AS production-workload-vmhost
RUN apk add --no-cache ca-certificates curl e2fsprogs iptables squashfs-tools \
    && command -v sqfstar mke2fs iptables-save iptables-restore
COPY --from=build /opt/dist /usr/bin
COPY --from=build-guest /opt/guest/workload-guest /usr/bin/workload-guest
COPY --from=firecracker /opt/workload-vmhost/bin/firecracker /usr/local/bin/firecracker
COPY --from=firecracker /opt/workload-vmhost/vmlinux /opt/workload-vmhost/vmlinux
ENV GODEBUG=gctrace=1
ENTRYPOINT [ "app" ]
CMD ["serve-workload-vmhost"]
