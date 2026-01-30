.PHONY: compose-up compose-down compose-logs compose-psql compose-reset

# Load .env if present so compose uses local values.
ifneq (,$(wildcard .env))
include .env
export
endif

POSTGRES_USER ?= spacescale
POSTGRES_DB ?= postgres
COMPOSE ?= docker compose -f docker-compose.yaml

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
