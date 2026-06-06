.PHONY: compose-start scalec proto lint test stop


compose-start:
	docker-compose up -d


scalec:
	go run ./cmd/scalecp


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


lint:
	golangci-lint run ./...


test:
	go test ./...


stop:
	docker-compose down
