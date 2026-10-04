# The microsandbox release the workload-microsandbox images are built on, named
# by version and pinned by the digest of its multi-platform index. It has to be
# the release of the SDK go.mod requires: an SDK and an msb of different
# versions break the database they share under MSB_HOME. So both are bumped in
# one change, after reading every changelog in between, and
# scripts/check-microsandbox-version.sh fails CI when they part.
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

# workload microsandbox service
#
# The one image that is not static. Microsandbox's SDK is a cgo binding to a
# library that needs glibc, so the service is built with cgo and the
# microsandbox tag, and runs on microsandbox's own image, which carries the msb
# and libkrunfw of the release the SDK belongs to. Bookworm's glibc (2.36) is
# new enough for the SDK and older than that image's (2.39), so the binary runs
# where it lands.
FROM golang:1.27-bookworm AS build-microsandbox
WORKDIR /opt/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags microsandbox -trimpath -buildvcs=false -o /opt/dist/app .

# tini is PID 1 and reaps the msb processes the service leaves behind. There is
# no supervisor besides it: when the service exits, tini does, the container
# stops, and every microVM goes with it, which is what restart: unless-stopped
# answers for.
FROM ghcr.io/superradcompany/microsandbox:${MICROSANDBOX_VERSION}@${MICROSANDBOX_DIGEST} AS production-workload-microsandbox
ARG MICROSANDBOX_LIBKRUNFW
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends tini curl ca-certificates procps \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 app \
    && useradd -u 10001 -g app -M -d /data -s /usr/sbin/nologin app \
    && mkdir -p /data \
    && chown app:app /data
COPY --from=build-microsandbox /opt/dist/app /usr/bin/app
USER 10001:10001
# MSB_HOME is short because microsandbox puts sockets under it, and a socket's
# path is limited to about 108 bytes. MSB_PATH and MSB_LIBKRUNFW_PATH name the
# image's own pair, so the SDK never fetches another.
ENV MSB_HOME=/data/msb \
    MSB_PATH=/usr/local/bin/msb \
    MSB_LIBKRUNFW_PATH=/usr/local/lib/${MICROSANDBOX_LIBKRUNFW}
EXPOSE 8443
ENTRYPOINT ["/usr/bin/tini", "--", "app"]
CMD ["serve-workload-microsandbox"]

# the same while developing: microsandbox's image, with Go and a C toolchain,
# rebuilt by air under the microsandbox tag. Its binary has a name of its own,
# because every service shares ./tmp and the others' is static.
FROM ghcr.io/superradcompany/microsandbox:${MICROSANDBOX_VERSION}@${MICROSANDBOX_DIGEST} AS develop-workload-microsandbox
ARG MICROSANDBOX_LIBKRUNFW
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends gcc libc6-dev curl ca-certificates procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=golang:1.27-bookworm /usr/local/go /usr/local/go
ENV GOPATH=/go \
    PATH=/go/bin:/usr/local/go/bin:$PATH \
    CGO_ENABLED=1 \
    MSB_HOME=/data/msb \
    MSB_PATH=/usr/local/bin/msb \
    MSB_LIBKRUNFW_PATH=/usr/local/lib/${MICROSANDBOX_LIBKRUNFW}
WORKDIR /opt/app
COPY go.mod go.sum ./
RUN go mod download
EXPOSE 8443
ENTRYPOINT ["go", "tool", "air", \
    "-build.cmd=go build -tags microsandbox -buildvcs=false -o ./tmp/workload-microsandbox .", \
    "-build.entrypoint=./tmp/workload-microsandbox", \
    "-build.poll=true", \
    "-build.poll_interval=2000", \
    "-build.include_ext=go,tmpl", \
    "-build.exclude_dir=tmp,vendor,testdata,.git,.github,resources/docs,storage", \
    "-build.exclude_regex=_test\\.go$", \
    "-build.stop_on_error=false", \
    "--"]
CMD ["serve-workload-microsandbox"]
