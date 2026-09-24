FROM golang:1.26-alpine AS base
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

# the agent every microVM boots as its init. It is kept apart from the
# application's own binary: it is unpacked into every machine's memory before
# anything runs there, so what it carries is what every machine pays for.
FROM base AS build-guest
RUN go build -v -o /opt/guest/runner-guest ./cmd/runner-guest

# what a launcher starts microVMs with, pinned, for the platform the image is
# built for: firecracker, and the kernel machines boot.
FROM alpine:latest AS firecracker
ARG TARGETARCH
ARG FIRECRACKER_VERSION=v1.17.0
ARG KERNEL_CI_VERSION=v1.15
ARG KERNEL_VERSION=6.1.155
RUN apk add --no-cache curl tar \
    && case "$TARGETARCH" in \
        amd64) arch=x86_64 ;; \
        arm64) arch=aarch64 ;; \
        *) echo "there is no firecracker for $TARGETARCH" && exit 1 ;; \
    esac \
    && release="https://github.com/firecracker-microvm/firecracker/releases/download/${FIRECRACKER_VERSION}" \
    && curl -fsSL "${release}/firecracker-${FIRECRACKER_VERSION}-${arch}.tgz" | tar -xz -C /tmp \
    && mkdir -p /opt/runner/bin \
    && cp "/tmp/release-${FIRECRACKER_VERSION}-${arch}/firecracker-${FIRECRACKER_VERSION}-${arch}" /opt/runner/bin/firecracker \
    && chmod 0755 /opt/runner/bin/firecracker \
    && curl -fsSLo /opt/runner/vmlinux "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/${KERNEL_CI_VERSION}/${arch}/vmlinux-${KERNEL_VERSION}"

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

# runner control plane service
FROM develop AS develop-runner-controlplane
EXPOSE 80
CMD ["serve-runner-controlplane", "--port=80"]

FROM production AS production-runner-controlplane
EXPOSE 80
CMD ["serve-runner-controlplane", "--port=80"]

# runner ingress service
FROM develop AS develop-runner-ingress
EXPOSE 80
CMD ["serve-runner-ingress", "--port=80"]

FROM production AS production-runner-ingress
EXPOSE 80
CMD ["serve-runner-ingress", "--port=80"]

# runner orchestrator service. It makes images into the roots microVMs boot
# (squashfs-tools) and their scratch disks (e2fsprogs), and boots each machine
# with the agent it was built with. A change to the agent takes an image built
# again, in development as well: what is reloaded is the orchestrator alone.
FROM develop AS develop-runner-orchestrator
RUN apk add --no-cache squashfs-tools e2fsprogs \
    && go build -o /usr/bin/runner-guest ./cmd/runner-guest
ENV RUNNER_ORCHESTRATOR_NAME=runner-orchestrator-01
EXPOSE 80
CMD ["serve-runner-orchestrator", "--port=80"]

FROM production AS production-runner-orchestrator
USER root
RUN apk add --no-cache squashfs-tools e2fsprogs
COPY --from=build-guest /opt/guest/runner-guest /usr/bin/runner-guest
USER app:app
ENV RUNNER_ORCHESTRATOR_NAME=runner-orchestrator-01
EXPOSE 80
CMD ["serve-runner-orchestrator", "--port=80"]

# runner launcher service. It runs as root inside its own container, which is
# given no privilege on the host, and starts every machine's firecracker as a
# user of that machine's own.
FROM develop AS develop-runner-launcher
RUN apk add --no-cache iptables
COPY --from=firecracker /opt/runner/bin/ /usr/local/bin/
COPY --from=firecracker /opt/runner/vmlinux /opt/runner/vmlinux
CMD ["serve-runner-launcher"]

FROM alpine:latest AS production-runner-launcher
RUN apk add --no-cache iptables
COPY --from=build /opt/dist /usr/bin
COPY --from=firecracker /opt/runner/bin/ /usr/local/bin/
COPY --from=firecracker /opt/runner/vmlinux /opt/runner/vmlinux
ENTRYPOINT [ "app" ]
CMD ["serve-runner-launcher"]
