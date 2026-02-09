.PHONY: compose-up compose-down compose-logs compose-psql \
	migrate-up migrate-down migrate-reset \
	migrate-up-test migrate-down-test coverage clean test

test:
	docker compose -f docker-compose.yaml up --build -d
	make migrate-up-test
	TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" cd apps/api && go test ./... -race -cover

compose-up:
	docker compose -f docker-compose.yaml up --build -d
	make migrate-up


compose-down:
	docker compose -f docker-compose.yaml down

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

migrate-up-test:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" up

migrate-down-test:
	goose -dir apps/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" down

coverage:
	TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" cd apps/api && go test ./... -coverprofile=../../coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
