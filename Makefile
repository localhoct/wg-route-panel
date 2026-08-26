.PHONY: build run test lint clean install

build:
	go build -o bin/panel ./cmd/panel

run:
	go run ./cmd/panel

test:
	go test -v ./...

lint:
	gofmt -s -w .
	go vet ./...

clean:
	rm -rf bin/

install: build
	sudo ./scripts/install.sh
