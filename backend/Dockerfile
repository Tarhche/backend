FROM golang:1.23-alpine AS base
WORKDIR /opt/app
COPY . .
ENV GOARCH=amd64 CGO_ENABLED=0
RUN go mod download

FROM base AS build
WORKDIR /opt/dist
RUN go build -v -o app . && chmod +x app

FROM base AS develop
WORKDIR /opt/app
ENV PATH=$GOPATH/bin/linux_$GOARCH:$PATH
RUN go install github.com/air-verse/air@v1.61 \
    && go install github.com/nats-io/natscli/nats@v0.1.5
EXPOSE 80
CMD ["air", "--", "serve", "-port=80"]

FROM alpine:latest AS production
COPY --from=build /opt/dist /usr/bin
ENV GODEBUG=gctrace=1
EXPOSE 80
CMD ["app", "serve", "-port=80"]
