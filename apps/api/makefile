# Build and test targets for the spacescale api
# Variables allow runtime configuration for run
# Test target runs unit tests
# Test docker target runs docker runtime tests
# Coverage target produces a report

.PHONY: build test test-race test-docker coverage clean run

ADDR ?= :8080
WORKER_TOKEN ?=
BASE_DOMAIN ?= localtest.me
TRAEFIK_NET ?= traefik
TRAEFIK_ENTRYPOINT ?= web
ENABLE_TLS ?= 0
RUN_DOCKER_TESTS ?= 1

build:
	go build -v ./...

run:
	ADDR=$(ADDR) WORKER_TOKEN=$(WORKER_TOKEN) BASE_DOMAIN=$(BASE_DOMAIN) TRAEFIK_NET=$(TRAEFIK_NET) TRAEFIK_ENTRYPOINT=$(TRAEFIK_ENTRYPOINT) ENABLE_TLS=$(ENABLE_TLS) go run ./cmd/api

test:
	RUN_DOCKER_TESTS=$(RUN_DOCKER_TESTS) go test ./...  -race -cover

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean:
	rm -f coverage.out coverage.html
