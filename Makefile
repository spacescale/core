.PHONY: db scalecp   proto lint  test stop


db:
	docker-compose up -d db nats
	@bash -euo pipefail -c 'until docker-compose exec -T db psql -U spacescale -d spacescale -c "select 1" >/dev/null 2>&1 && docker-compose exec -T db psql -U spacescale -d spacescale_test -c "select 1" >/dev/null 2>&1; do sleep 1; done'
	goose -dir scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" up
	goose -dir scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" up


scalecp: db
	@echo "Starting scalecp..."
	set -a && . ./.env.local && set +a && : "$${DATABASE_URL:?DATABASE_URL missing in .env.local}" && go run ./cmd/scalecp


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


lint:
	golangci-lint run ./...


test:
	go test ./...


stop:
	docker-compose down -v
