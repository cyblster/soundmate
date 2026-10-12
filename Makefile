.PHONY: help test lint docker-test docker-build up down logs

help:
	@echo "make test          run unit tests with coverage"
	@echo "make lint          run gofmt and go vet"
	@echo "make docker-test   run the test stage of the Docker build"
	@echo "make docker-build  build the Docker image (runs tests)"
	@echo "make up            build and start the whole stack"
	@echo "make down          stop the stack"
	@echo "make logs          follow bot logs"

test:
	go test -count=1 -cover ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

docker-test:
	docker build --progress=plain --target test .

docker-build:
	docker compose build --progress=plain soundmate-bot

up: docker-build
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f soundmate-bot
