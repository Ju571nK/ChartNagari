.PHONY: all build build-frontend build-all test test-coverage lint run docker-build docker-up docker-down docker-logs clean

BINARY  := ./chart-analyzer
WEB_DIR := ./web

all: build

build:
	go build -o $(BINARY) ./cmd/server

build-frontend:
	cd $(WEB_DIR) && npm install && npm run build

build-all: build-frontend build

test:
	go test ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	go vet ./...

run: build
	$(BINARY)

docker-build: build-frontend
	docker build -t chartter:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -rf $(WEB_DIR)/dist
