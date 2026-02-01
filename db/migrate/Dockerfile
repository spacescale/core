FROM golang:1.25-alpine

RUN apk add --no-cache ca-certificates git

RUN GOBIN=/usr/local/bin go install github.com/pressly/goose/v3/cmd/goose@latest

ENTRYPOINT ["goose"]
