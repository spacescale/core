FROM golang:1.26-alpine AS goose-builder
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

FROM postgres:18-alpine
ENV POSTGRES_USER=spacescale
ENV POSTGRES_PASSWORD=spacescale
ENV POSTGRES_DB=postgres
COPY --from=goose-builder /go/bin/goose /usr/local/bin/goose
COPY init/ /docker-entrypoint-initdb.d/
COPY migrations/ /migrations/
RUN cat > /usr/local/bin/start-with-migrations.sh <<'EOF' && chmod +x /usr/local/bin/start-with-migrations.sh
#!/usr/bin/env sh
set -eu

rm -f /tmp/migrations.done

docker-entrypoint.sh postgres &
pg_pid="$!"

cleanup() {
  kill -TERM "$pg_pid" >/dev/null 2>&1 || true
  wait "$pg_pid" >/dev/null 2>&1 || true
}
trap cleanup INT TERM

until pg_isready -U "${POSTGRES_USER}" -d postgres >/dev/null 2>&1; do
  sleep 1
done

until psql -U "${POSTGRES_USER}" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='spacescale'" | grep -q 1; do
  sleep 1
done

until psql -U "${POSTGRES_USER}" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='spacescale_test'" | grep -q 1; do
  sleep 1
done

goose -dir /migrations postgres "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/spacescale?sslmode=disable" up
goose -dir /migrations postgres "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/spacescale_test?sslmode=disable" up

touch /tmp/migrations.done

wait "$pg_pid"
EOF

EXPOSE 5432
CMD ["start-with-migrations.sh"]
# db dockerfile is intentionally this way its for locL dev and not for production purposes