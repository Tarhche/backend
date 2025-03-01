FROM golang:1.23-alpine AS base
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
RUN apk add tmux \
    && go install github.com/air-verse/air@v1.61 \
    && go install github.com/nats-io/natscli/nats@v0.1.6
EXPOSE 80
CMD ["air", "--", "serve", "-port=80"]

FROM alpine:latest AS production
COPY --from=build /opt/dist /usr/bin
ENV GODEBUG=gctrace=1
EXPOSE 80
CMD ["app", "serve", "-port=80"]
