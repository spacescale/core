.PHONY: db cp provision-node scaled view-bootstrap-token view-identity proto test stop


db:
	docker compose up -d db nats
	@bash -euo pipefail -c 'until docker compose exec -T db pg_isready -U spacescale -d postgres >/dev/null 2>&1; do sleep 1; done'
	goose -dir internal/scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable" up
	goose -dir internal/scalecp/db/migrations postgres "postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" up


cp: db
	@echo "Starting scalecp..."
	set -a && . ./.env.local && set +a && : "$${DATABASE_URL:?DATABASE_URL missing in .env.local}" && go run ./cmd/scalecp


provision-node: db
	./scripts/dev/provision-manual-node.sh


scaled:
	ssh root@65.109.67.102 'bash -lc /usr/local/bin/scaled'


view-bootstrap-token:
	ssh root@65.109.67.102 'cat /var/lib/spacescale/bootstrap_token'


view-identity:
	ssh root@65.109.67.102 'cat /var/lib/spacescale/identity.json'


proto:
	protoc --proto_path=. --go_out=. --go_opt=module=github.com/spacescale/core $$(find proto -type f -name '*.proto' | sort)


test: db
	TEST_DATABASE_URL="postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable" go test ./... -race


stop:
	docker compose down --remove-orphans -v
