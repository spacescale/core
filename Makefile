.PHONY: compose-up compose-down compose-reset compose-logs compose-psql \
	migrate-up migrate-down migrate-reset \
	goose-create \
	migrate-up-test migrate-down-test coverage clean test


## Support positional usage: `make goose-create add_users_table`.
## Make treats extra words as additional targets, so we:
## 1) capture the 2nd word as migration name,
## 2) fail fast when it's missing,
## 3) register that word as a no-op target to avoid "No rule to make target".
ifneq (,$(filter goose-create,$(MAKECMDGOALS)))
MIGRATION_NAME := $(word 2,$(MAKECMDGOALS))
ifeq ($(MIGRATION_NAME),)
$(error Usage: make goose-create <name>)
endif
$(eval $(MIGRATION_NAME):;@:)
endif

test:
	docker compose -f docker-compose.yaml up --build -d
	make migrate-up-test
	cd apps/api && TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" cd apps/api && go test ./... -race -cover

compose-up:
	docker compose -f docker-compose.yaml up --build -d
	make migrate-up-test
	make migrate-up


compose-down:
	docker compose -f docker-compose.yaml down

compose-reset:
	docker compose -f docker-compose.yaml down -v

compose-logs:
	docker compose -f docker-compose.yaml logs -f --tail=200

compose-psql:
	docker compose -f docker-compose.yaml exec db psql -U spacescale -d spacescale

migrate-up:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" up

migrate-down:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" down

migrate-reset:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" reset

goose-create:
	goose -dir apps/db/migrations create $(MIGRATION_NAME) sql

migrate-up-test:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" up

migrate-down-test:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" down

coverage:
	cd apps/api && TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" cd apps/api && go test ./... -coverprofile=../../coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
