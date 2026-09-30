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
