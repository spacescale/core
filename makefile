.PHONY: build test test-race coverage clean run

build:
	go build -v ./...

run:
	go run ./cmd/api

test:
	go test ./... -cover

test-race:
	go test ./... -race

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
