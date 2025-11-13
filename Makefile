APP_NAME=reviewer-service
DB_SERVICE=db

.PHONY: build run migrate docker-up docker-down

build:
	go build -o $(APP_NAME) ./cmd

run:
	go run ./cmd

