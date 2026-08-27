SHELL := /bin/bash
BINARY := bin/panel
.PHONY: build run test lint clean install start doctor docker-build docker-up docker-down docker-logs
build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/panel
run:
	PANEL_CONFIG_PATH=configs/panel.local.yaml go run ./cmd/panel serve
test:
	go test -race ./...
lint:
	test -z "$$(gofmt -l .)" || (gofmt -d .; exit 1)
	go vet ./...
clean:
	rm -rf bin data
install: build
	sudo ./scripts/install.sh
start:
	./scripts/start.sh
doctor:
	./scripts/doctor.sh
docker-build:
	docker compose build
docker-up:
	docker compose up -d --build
docker-down:
	docker compose down
docker-logs:
	docker compose logs -f panel
