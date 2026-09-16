.PHONY: build run lint test

IMAGE := primosa-uptime

build:
	docker build -t $(IMAGE) .

run: build
	docker run --rm -v "$(PWD):/data" -w /data \
		-e TELEGRAM_BOT_TOKEN -e TELEGRAM_CHAT_ID \
		$(IMAGE)

lint: test
	test -z "$$(gofmt -l .)"
	go vet ./...

test:
	docker run --rm -v "$(PWD):/src" -w /src golang:1.26-alpine go test ./...
