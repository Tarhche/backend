# The microsandbox release the vmhost image is built on, named by version and
# pinned by the digest of its multi-platform index. It has to be the release
# of the SDK go.mod requires: an SDK and an msb of different releases break
# the database they share under MSB_HOME, so both are bumped in one change,
# and the engine's Version with them.
ARG MICROSANDBOX_VERSION=0.7.6
ARG MICROSANDBOX_DIGEST=sha256:be3d9f5f99b937ac4c4452b2ff33657a4ef74a117fdb584abb6d3c7db53e248f
# the guest kernel library that release ships, the same on amd64 and arm64
ARG MICROSANDBOX_LIBKRUNFW=libkrunfw.so.5.6.1

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

# workload vmhost service
#
# The one image that is not static. microsandbox's SDK is a cgo binding to a
# library that needs glibc, so the vmhost is built with cgo and the
# microsandbox tag, on bookworm: its glibc (2.36) is new enough for the SDK and
# older than the microsandbox image's (2.39), so the binary runs where it lands.
FROM golang:1.27-bookworm AS build-workload-vmhost
WORKDIR /opt/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -tags microsandbox -trimpath -buildvcs=false -o /opt/dist/app .

# microsandbox's own image, which carries the msb and libkrunfw of the release
# the SDK belongs to; MSB_PATH and MSB_LIBKRUNFW_PATH name that pair, so the
# SDK never fetches another. MSB_HOME is short because microsandbox puts
# sockets under it, and a socket's path is limited to about 108 bytes.
#
# burn-pids.sh runs before the vmhost and becomes it (#1642), and needs
# sqlite3 to read what microsandbox recorded. Nothing here reaps the msb
# processes the vmhost leaves behind: the container is run with an init that
# does, and with /dev/kvm, the kvm group and no capabilities.
FROM ghcr.io/superradcompany/microsandbox:${MICROSANDBOX_VERSION}@${MICROSANDBOX_DIGEST} AS production-workload-vmhost
ARG MICROSANDBOX_LIBKRUNFW
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends sqlite3 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 app \
    && useradd -u 10001 -g app -M -d /data -s /usr/sbin/nologin app \
    && mkdir -p /data/msb \
    && chown -R app:app /data
COPY --from=build-workload-vmhost /opt/dist/app /usr/bin/app
COPY scripts/vmhost/burn-pids.sh /usr/local/bin/burn-pids.sh
USER 10001:10001
ENV HOME=/data \
    MSB_HOME=/data/msb \
    MSB_PATH=/usr/local/bin/msb \
    MSB_LIBKRUNFW_PATH=/usr/local/lib/${MICROSANDBOX_LIBKRUNFW}
ENTRYPOINT ["/usr/local/bin/burn-pids.sh", "app"]
CMD ["serve-workload-vmhost"]
