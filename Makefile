.PHONY: build run tidy clean dev

APP=flight-collaboration-platform
BIN=./bin

build:
	@mkdir -p $(BIN)
	go build -o $(BIN)/$(APP) ./cmd/server

run:
	go run ./cmd/server -config configs/config.yaml

dev: run

tidy:
	go mod tidy

clean:
	rm -rf $(BIN)

test:
	go test ./... -v
