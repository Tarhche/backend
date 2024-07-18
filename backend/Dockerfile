FROM golang:1.22 as build
WORKDIR /dist
COPY . .
ENV GOARCH=amd64 CGO_ENABLED=0
RUN go mod download
RUN go build -v -o app ./main.go && chmod +x app

FROM alpine:latest as production
COPY --from=build /dist /usr/bin
EXPOSE 80
CMD ["app", "serve", "-port=80"]