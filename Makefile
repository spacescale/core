.PHONY: compose-up compose-down compose-reset compose-logs compose-psql \
	migrate-up migrate-down migrate-reset \
	goose-create \
	migrate-up-test migrate-down-test coverage clean test mint

DB_URL ?= postgres://spacescale:spacescale@localhost:5432/spacescale?sslmode=disable
TEST_DB_URL ?= postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable


## Support positional usage: `make goose-create add_users_table`.
## Make treats extra words as additional targets, so we:
##  capture the 2nd word as migration name,
##  fail fast when it's missing,
##  register that word as a no-op target to avoid "No rule to make target".
ifneq (,$(filter goose-create,$(MAKECMDGOALS)))
MIGRATION_NAME := $(word 2,$(MAKECMDGOALS))
ifeq ($(MIGRATION_NAME),)
$(error Usage: make goose-create <name>)
endif
$(eval $(MIGRATION_NAME):;@:)
endif

test:
	make compose-reset ## removes volume so database tests wont crash
	docker compose -f docker-compose.yaml up --build -d
	make migrate-up-test
	cd apps/api && TEST_DATABASE_URL="$(TEST_DB_URL)" go test ./internal/http_api ./internal/service -race -cover

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
	goose -dir apps/db/migrations postgres "$(DB_URL)" up

migrate-down:
	goose -dir apps/db/migrations postgres "$(DB_URL)" down

migrate-reset:
	goose -dir apps/db/migrations postgres "$(DB_URL)" reset

goose-create:
	goose -dir apps/db/migrations create $(MIGRATION_NAME) sql

migrate-up-test:
	goose -dir apps/db/migrations postgres "$(TEST_DB_URL)" up

migrate-down-test:
	goose -dir apps/db/migrations postgres "$(TEST_DB_URL)" down

coverage:
	cd apps/api && TEST_DATABASE_URL="$(TEST_DB_URL)" go test ./internal/http_api ./internal/service -coverprofile=../../coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@if command -v open >/dev/null 2>&1; then open coverage.html; else echo "coverage report: coverage.html"; fi

random-hash:
	# generates 32 random bytes and prints them as hex
	openssl rand -hex 32

## Mint a BFF JWT for local API testing.
## Reads values from .env.local.
## Required keys:
## - BFF_JWT_SECRET
## - BFF_JWT_SUB (format: github:<id>)
## Optional keys:
## - BFF_JWT_ISSUER (default: spacescale-web-bff)
## - BFF_JWT_AUDIENCE (default: spacescale-api)
## - BFF_JWT_TTL_SECONDS (default: 3600)
## - BFF_JWT_EMAIL, BFF_JWT_NAME, BFF_JWT_AVATAR_URL
mint:
	@node -e 'const fs=require("fs"); \
	const crypto=require("crypto"); \
	const envPath=".env.local"; \
	if(!fs.existsSync(envPath)){console.error(".env.local not found in repo root");process.exit(1);} \
	const parseEnv=(text)=>{const out={};for(const raw of text.split(/\r?\n/)){const line=raw.trim();if(!line||line.startsWith("#")) continue;const idx=line.indexOf("=");if(idx===-1) continue;const key=line.slice(0,idx).trim();let val=line.slice(idx+1).trim();if(val.startsWith("\"")&&val.endsWith("\"")){val=val.slice(1,-1);}out[key]=val;}return out;}; \
	const env=parseEnv(fs.readFileSync(envPath,"utf8")); \
	const secret=(env.BFF_JWT_SECRET||"").trim(); \
	const subject=(env.BFF_JWT_SUB||"").trim(); \
	if(!secret){console.error("BFF_JWT_SECRET is required in .env.local");process.exit(1);} \
	if(!subject){console.error("BFF_JWT_SUB is required in .env.local");process.exit(1);} \
	if(!subject.startsWith("github:")||subject.length<8){console.error("BFF_JWT_SUB must use format github:<id>");process.exit(1);} \
	const issuer=(env.BFF_JWT_ISSUER||"spacescale-web-bff").trim(); \
	const audience=(env.BFF_JWT_AUDIENCE||"spacescale-api").trim(); \
	const ttl=Number(env.BFF_JWT_TTL_SECONDS||"3600"); \
	if(!Number.isFinite(ttl)||ttl<=0){console.error("BFF_JWT_TTL_SECONDS must be a positive number");process.exit(1);} \
	const now=Math.floor(Date.now()/1000); \
	const payload={sub:subject,iss:issuer,aud:audience,iat:now,exp:now+ttl}; \
	if(env.BFF_JWT_EMAIL){payload.email=env.BFF_JWT_EMAIL;} \
	if(env.BFF_JWT_NAME){payload.name=env.BFF_JWT_NAME;} \
	if(env.BFF_JWT_AVATAR_URL){payload.avatar_url=env.BFF_JWT_AVATAR_URL;} \
	const b64=(v)=>Buffer.from(JSON.stringify(v)).toString("base64url"); \
	const header={alg:"HS256",typ:"JWT"}; \
	const input=b64(header)+"."+b64(payload); \
	const sig=crypto.createHmac("sha256",secret).update(input).digest("base64url"); \
	process.stdout.write(input+"."+sig+"\n");'

clean:
	rm -f coverage.out coverage.html
