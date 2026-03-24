.PHONY: build docker lint test test-integration

build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nginx-clickhouse .

docker:
	docker build --rm --no-cache=true -t mintance/nginx-clickhouse -f Dockerfile .

lint:
	gofmt -l .
	go vet ./...

test:
	go test ./... -v -race

test-integration:
	go test ./clickhouse/ -v -race -tags integration
	go test . -v -race -tags integration -timeout 120s
