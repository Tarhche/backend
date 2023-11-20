FROM golang:1.21 as build

WORKDIR /dist
COPY . .

# Adjust the ARCH if needed - eg amd64 or arm64v8
ENV GOARCH=amd64 CGO_ENABLED=0

RUN go mod download

# Build the binary
RUN go build -v -o app ./main.go && chmod +x app

FROM alpine:latest as production

COPY --from=build /dist /usr/bin

CMD ["app", "serve", "-port=80"]