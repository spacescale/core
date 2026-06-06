.PHONY: compose-start controlp build-scaled proto lint test stop


compose-start:
	docker-compose up -d


controlp:
	go run ./cmd/controlp


build-scaled:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/scaled ./cmd/scaled


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


lint:
	golangci-lint run ./...


test:
	go test ./...


stop:
	docker-compose down
