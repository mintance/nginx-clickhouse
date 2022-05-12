GolangVersion = 1.17.1-buster
GOAMD64 = v3

build:
	go mod vendor
	CGO_ENABLED=0 GOOS=linux GOAMD64=$(GOAMD64) go build -a -installsuffix cgo -o nginx-clickhouse .

docker:
	docker build --rm --no-cache=true -e GOAMD64=$(GOAMD64) -t mintance/nginx-clickhouse -f Dockerfile .