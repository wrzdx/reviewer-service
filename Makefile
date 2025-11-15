APP_NAME=reviewer-service
DB_SERVICE=db

.PHONY: build run docker-up docker-down test lint

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

test:
	docker compose up --build -d db app go-tester
	sleep 10
	docker compose exec go-tester go test -v ./app/db/...
	docker compose exec go-tester go test -v ./app/e2e/...
	docker compose down -v

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run ./... --fix