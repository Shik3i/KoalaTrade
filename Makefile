# KoalaTrade — Development Makefile

.PHONY: dev-backend dev-frontend build-backend build-frontend docker-up docker-down

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

build-backend:
	cd backend && go build -o server ./cmd/server

build-frontend:
	cd frontend && npm run build

docker-up:
	docker compose up --build

docker-down:
	docker compose down
