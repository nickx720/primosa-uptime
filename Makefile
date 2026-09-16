.PHONY: build run lint test

IMAGE := primosa-uptime

build:
	docker build -t $(IMAGE) .

run: build
	docker run --rm -v "$(PWD):/data" -w /data \
		-e TELEGRAM_BOT_TOKEN -e TELEGRAM_CHAT_ID \
		$(IMAGE)

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

test: build
	docker run --rm -v "$(PWD):/data" -w /data $(IMAGE)
