.PHONY: db-build db-start run test stop mint

db-build:
	docker build -f apps/db/Dockerfile -t spacescale-db:local apps/db

db-start: db-build
	@docker rm -f spacescale-db || true
	docker run --name spacescale-db -e POSTGRES_PASSWORD=spacescale -p 5432:5432 -d spacescale-db:local
	@bash -euo pipefail -c 'until docker exec spacescale-db test -f /tmp/migrations.done; do sleep 1; done; '

run: db-start
	@[ -f .env.local ] || { echo ".env.local not found in repo root"; exit 1; };
	pid=$$(lsof -tiTCP:8080 -sTCP:LISTEN || true); [ -z "$$pid" ] || kill -TERM $$pid;
	set -a && . ./.env.local && set +a && : "$${DATABASE_URL:?DATABASE_URL missing in .env.local}" && cd apps/api && go run .

test: db-start
	@[ -f .env.local ] || { echo ".env.local not found in repo root"; exit 1; };
	set -a && . ./.env.local && set +a && cd apps/api && TEST_DATABASE_URL="$${TEST_DATABASE_URL:-postgres://spacescale:spacescale@localhost:5432/spacescale_test?sslmode=disable}" go test ./internal/http_api ./internal/service -race -cover

proto-go:
	protoc --proto_path=. --go_out=apps/api --go_opt=module=github.com/t0gun/spacescale --go-grpc_out=apps/api --go-grpc_opt=module=github.com/t0gun/spacescale $$(find contracts/proto -type f -name '*.proto' | sort)

stop:
	@docker rm -f spacescale-db || true

## Mint a BFF JWT for local API testing.
## Reads values from .env.local.
## Required keys:
## - BFF_JWT_SECRET
## - BFF_IDENTITY_KEY (stable identity key used by API)
##   (legacy fallback: BFF_JWT_SUB)
## Optional keys:
## - BFF_JWT_ISSUER (default: spacescale-web-bff)
## - BFF_JWT_AUDIENCE (default: spacescale-api)
## - BFF_JWT_TTL_SECONDS (default: 3600)
## - BFF_SUBJECT_HASH_SECRET or NEXTAUTH_SECRET (subject hash source)
## - BFF_JWT_EMAIL, BFF_JWT_NAME, BFF_JWT_AVATAR_URL
mint:
	@node -e 'const fs=require("fs"); \
	const crypto=require("crypto"); \
	const envPath=".env.local"; \
	if(!fs.existsSync(envPath)){console.error(".env.local not found in repo root");process.exit(1);} \
	const parseEnv=(text)=>{const out={};for(const raw of text.split(/\r?\n/)){const line=raw.trim();if(!line||line.startsWith("#")) continue;const idx=line.indexOf("=");if(idx===-1) continue;const key=line.slice(0,idx).trim();let val=line.slice(idx+1).trim();if(val.startsWith("\"")&&val.endsWith("\"")){val=val.slice(1,-1);}out[key]=val;}return out;}; \
	const env=parseEnv(fs.readFileSync(envPath,"utf8")); \
	const secret=(env.BFF_JWT_SECRET||"").trim(); \
	const identityKey=(env.BFF_IDENTITY_KEY||env.BFF_JWT_SUB||"").trim(); \
	const subjectHashSecret=(env.BFF_SUBJECT_HASH_SECRET||env.NEXTAUTH_SECRET||secret).trim(); \
	if(!secret){console.error("BFF_JWT_SECRET is required in .env.local");process.exit(1);} \
	if(!identityKey){console.error("BFF_IDENTITY_KEY is required in .env.local");process.exit(1);} \
	const subject="github:v2:"+crypto.createHmac("sha256",subjectHashSecret).update(identityKey).digest("hex"); \
	const issuer=(env.BFF_JWT_ISSUER||"spacescale-web-bff").trim(); \
	const audience=(env.BFF_JWT_AUDIENCE||"spacescale-api").trim(); \
	const ttl=Number(env.BFF_JWT_TTL_SECONDS||"3600"); \
	if(!Number.isFinite(ttl)||ttl<=0){console.error("BFF_JWT_TTL_SECONDS must be a positive number");process.exit(1);} \
	const now=Math.floor(Date.now()/1000); \
	const payload={sub:subject,identity_key:identityKey,iss:issuer,aud:audience,iat:now,exp:now+ttl}; \
	if(env.BFF_JWT_EMAIL){payload.email=env.BFF_JWT_EMAIL;} \
	if(env.BFF_JWT_NAME){payload.name=env.BFF_JWT_NAME;} \
	if(env.BFF_JWT_AVATAR_URL){payload.avatar_url=env.BFF_JWT_AVATAR_URL;} \
	const b64=(v)=>Buffer.from(JSON.stringify(v)).toString("base64url"); \
	const header={alg:"HS256",typ:"JWT"}; \
	const input=b64(header)+"."+b64(payload); \
	const sig=crypto.createHmac("sha256",secret).update(input).digest("base64url"); \
	process.stdout.write(input+"."+sig+"\n");'
