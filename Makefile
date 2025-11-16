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
	docker compose -f docker-compose.yml -f docker-compose.test.yml up --build -d db app go-tester
	sleep 10
	docker compose -f docker-compose.yml -f docker-compose.test.yml exec go-tester go test -v -p 1 ./app/db/... ./app/e2e/...
	docker compose -f docker-compose.yml -f docker-compose.test.yml down -v

test-load:
	docker compose -f docker-compose.yml -f docker-compose.test.yml up -d db app
	sleep 10
	docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm k6 run /app/load_test.js
	docker compose -f docker-compose.yml -f docker-compose.test.yml down -v

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run ./... --fix