SHELL := /bin/bash
BINARY := bin/panel
.PHONY: build run test lint clean install start doctor docker-build docker-up docker-down docker-logs vendor
build:
	go build -mod=vendor -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/panel
run:
	PANEL_CONFIG_PATH=configs/panel.local.yaml go run -mod=vendor ./cmd/panel serve
test:
	go test -mod=vendor -race ./...
lint:
	test -z "$$(gofmt -l $$(go list -f '{{.Dir}}' ./...))" || (gofmt -d $$(go list -f '{{.Dir}}' ./...); exit 1)
	go vet -mod=vendor ./...
# Dependencies are vendored in ./vendor and committed to the repo so the
# Docker build never needs network access to proxy.golang.org. Run this
# after adding/upgrading a Go dependency (go get ...) and commit the result.
vendor:
	go mod tidy
	go mod vendor
	go build -mod=vendor ./...
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
