APP_NAME=reviewer-service
DB_SERVICE=db

.PHONY: build run docker-up docker-down logs

# Build the Go application
build:
	go build -o $(APP_NAME) ./cmd

# Run the Go application not in Docker
run:
	go run ./cmd

# Run the application with Docker Compose
docker-up:
	docker compose up --build -d

# Stop the application and remove containers
docker-down:
	docker compose down

