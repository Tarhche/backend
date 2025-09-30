FROM golang:1.25-alpine AS base
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
ENTRYPOINT ["go", "tool", "air", "--"]

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
CMD ["serve-blog", "-port=80"]

FROM production AS production-blog
EXPOSE 80
CMD ["serve-blog", "-port=80"]

# runner manager service
FROM develop AS develop-runner-manager
EXPOSE 80
CMD ["serve-runner-manager", "-port=80"]

FROM production AS production-runner-manager
EXPOSE 80
CMD ["serve-runner-manager", "-port=80"]

# runner worker service
FROM develop AS develop-runner-worker
ENV RUNNER_WORKER_NAME=runner-worker-01
EXPOSE 80
CMD ["serve-runner-worker", "-port=80"]

FROM production AS production-runner-worker
ENV RUNNER_WORKER_NAME=runner-worker-01
EXPOSE 80
CMD ["serve-runner-worker", "-port=80"]
