# build stage
FROM golang:1.25 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app ./cmd/api

# runtime
FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
