.PHONY: build test coverage clean

build:
	go build -v ./...

test:
	go test ./... -cover

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
