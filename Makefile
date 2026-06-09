.PHONY: compose-start controlp build-scaled clean-dist proto lint test coverage stop ssh


compose-start:
	docker compose up -d controlp


controlp:
	docker compose up controlp


build-scaled:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/scaled ./cmd/scaled


clean-dist:
	rm -rf dist


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


lint:
	golangci-lint run ./...


test:
	go test ./...


coverage:
	docker compose up coverage
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html


stop:
	docker compose down

stop-with-volume:
	docker 	compose down -v

ssh:
	ssh ubuntu@4.249.148.167
