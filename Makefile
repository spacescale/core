.PHONY: compose-up compose-down compose-logs compose-psql compose-reset \
	migrate-up migrate-down migrate-status migrate-reset \
	migrate-up-test migrate-down-test migrate-status-test migrate-create

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

compose-up:
	$(COMPOSE) up --build -d

compose-down:
	$(COMPOSE) down

compose-logs:
	$(COMPOSE) logs -f --tail=200

compose-psql:
	$(COMPOSE) exec db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

compose-reset:
	$(COMPOSE) down -v

migrate-up:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" up

migrate-down:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" down

migrate-status:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" status

migrate-reset:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(DATABASE_URL)" reset

migrate-up-test:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(TEST_DATABASE_URL)" up

migrate-down-test:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(TEST_DATABASE_URL)" down

migrate-status-test:
	$(COMPOSE) --profile migrate run --rm migrate -dir /migrations postgres "$(TEST_DATABASE_URL)" status

migrate-create:
	$(GOOSE) -dir $(MIGRATIONS_DIR) create $(NAME) sql
