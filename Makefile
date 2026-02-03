.PHONY: compose-up compose-down compose-logs compose-psql compose-reset \
	migrate-up migrate-down migrate-status migrate-reset \
	migrate-up-test migrate-down-test migrate-status-test migrate-create coverage clean  test

# Load .env if present so compose uses local values.
ifneq (,$(wildcard .env))
include .env
export
endif

POSTGRES_USER ?= spacescale
POSTGRES_DB ?= postgres
COMPOSE ?= docker compose -f docker-compose.yaml
GOOSE ?= goose
MIGRATIONS_DIR ?= db/migrations

test:
	cd api &&  go test ./... -race -cover

compose-up:
	$(COMPOSE) up --build -d
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" up


compose-down:
	$(COMPOSE) down

compose-logs:
	$(COMPOSE) logs -f --tail=200

compose-psql:
	$(COMPOSE) exec db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

migrate-up:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" up

migrate-down:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" down

migrate-reset:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" reset

migrate-up-test:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(TEST_DATABASE_URL)" up

migrate-down-test:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(TEST_DATABASE_URL)" down

coverage:
	cd api && go test ./... -coverprofile=../coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
