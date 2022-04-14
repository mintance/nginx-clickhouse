build:
	go mod vendor
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nginx-clickhouse .

docker:
	docker build --rm --no-cache=true -t mintance/nginx-clickhouse -f Dockerfile .