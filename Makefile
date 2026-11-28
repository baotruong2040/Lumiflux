.PHONY: up down migrate-up migrate-down dev-api dev-web dev-tauri build-tauri test-api test-integration lint-web build-web seed loadtest

COMPOSE_FILE := discord-clone-api/deployments/docker-compose.yml
DB_URL ?= postgres://discord:discord_secret_dev@localhost:5432/discord_clone?sslmode=disable
LOADTEST_CLIENTS ?= 100
LOADTEST_MESSAGES ?= 20

up:
	docker compose -f $(COMPOSE_FILE) up -d

down:
	docker compose -f $(COMPOSE_FILE) down

migrate-up:
	cd discord-clone-api && migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	cd discord-clone-api && migrate -path migrations -database "$(DB_URL)" down 1

dev-api:
	cd discord-clone-api && go run ./cmd/api

dev-web:
	cd discord-clone-app && npm run dev

dev-tauri:
	cd discord-clone-app && npx tauri dev

build-tauri:
	cd discord-clone-app && npx tauri build

test-api:
	cd discord-clone-api && go vet ./... && go test -race -count=1 ./...

test-integration:
	cd discord-clone-api && RUN_INTEGRATION_TESTS=1 go test -v -race -count=1 ./internal/integration/...

lint-web:
	cd discord-clone-app && npm run lint

build-web:
	cd discord-clone-app && npm run build

seed:
	cd discord-clone-api && go run ./cmd/seed

loadtest:
	cd discord-clone-api && go run ./cmd/loadtest -clients $(LOADTEST_CLIENTS) -messages $(LOADTEST_MESSAGES)
