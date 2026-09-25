.PHONY: build test run seed deploy

build:
	go build -o bin/sidebet ./cmd/sidebet

test:
	go vet ./...
	go test -race ./...

run: build
	./bin/sidebet -addr 127.0.0.1:8090 -db sidebet.db

seed:
	python3 scripts/seed.py http://127.0.0.1:8090

deploy:
	fly deploy
