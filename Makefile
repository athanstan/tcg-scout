APP := tcg-scout

.PHONY: build run serve test tidy

build:
	go build -o bin/$(APP) ./cmd/$(APP)

run:
	go run ./cmd/$(APP)

serve:
	go run ./cmd/$(APP) serve

test:
	go test ./...

tidy:
	go mod tidy
