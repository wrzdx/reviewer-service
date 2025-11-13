APP_NAME=reviewer-service
DB_SERVICE=db

.PHONY: build run migrate docker-up docker-down

build:
	go build -o $(APP_NAME) ./cmd/server

run:
	go run ./cmd/server

migrate:
	go run ./cmd/migrate

docker-up:
	docker compose up -d $(DB_SERVICE)

docker-down:
	docker compose down
