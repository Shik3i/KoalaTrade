# KoalaTrade Makefile

.PHONY: dev-backend dev-frontend build run-backend docker-up

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

build:
	cd backend && go build -o server ./cmd/server

run-backend:
	cd backend && ./server

docker-up:
	docker compose up --build

docker-down:
	docker compose down
