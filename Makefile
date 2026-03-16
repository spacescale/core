.PHONY: db-start run run-scaled test stop mint proto secret


db-start:
	@docker compose down --remove-orphans >/dev/null 2>&1 || true
	docker compose up -d db
	docker compose up -d nats
	@bash -euo pipefail -c 'until docker compose exec -T db pg_isready -U spacescale -d postgres >/dev/null 2>&1; do sleep 1; done; '
	goose -dir internal/scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" up
	goose -dir internal/scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" up


run: db-start
	@[ -f .env.local ] || { echo ".env.local not found in repo root"; exit 1; };
	pid_api=$$(lsof -tiTCP:8080 -sTCP:LISTEN || true); [ -z "$$pid_api" ] || kill -TERM $$pid_api;
	@echo "Starting scalecp..."
	set -a && . ./.env.local && set +a && : "$${DATABASE_URL:?DATABASE_URL missing in .env.local}" && go run ./cmd/scalecp


run-scaled:
	@[ -f .env.local ] || { echo ".env.local not found in repo root"; exit 1; };
	@echo "Starting Scaled..."
	set -a && . ./.env.local && set +a && go run ./cmd/scaled


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


test: db-start
	TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" go test ./... -race


stop:
	@docker rm -f spacescale-db >/dev/null 2>&1 || true
	docker compose down --remove-orphans
