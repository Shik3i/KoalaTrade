# KoalaTrade — Development Makefile

ifneq (,$(wildcard .env))
    include .env
    export
endif

.PHONY: dev-backend dev-frontend test-backend check-frontend build-backend build-frontend docker-build docker-up docker-down ci

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

test-backend:
	cd backend && go test ./...

check-frontend:
	cd frontend && npm run check && npm run build

build-backend:
	cd backend && go build -o server ./cmd/server

build-frontend:
	cd frontend && npm run build

docker-build:
	docker build -f Dockerfile.backend -t koalatrade-backend:local .
	docker build -f Dockerfile.frontend -t koalatrade-frontend:local .

docker-up:
	docker compose up --build

docker-down:
	docker compose down

ci: test-backend check-frontend
