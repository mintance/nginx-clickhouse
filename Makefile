.PHONY: build docker lint test test-integration test-e2e validate-k8s-manifests

build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nginx-clickhouse .

docker:
	docker build --rm --no-cache=true -t mintance/nginx-clickhouse -f Dockerfile .

lint:
	test -z "$$(gofmt -l . | grep -v '^vendor/')"
	go vet ./...

test:
	go test ./... -v -race

test-integration:
	go test ./clickhouse/ -v -race -tags integration

test-e2e:
	go test . -v -race -tags e2e -timeout 120s

validate-k8s-manifests:
	kubeconform -summary -strict -ignore-missing-schemas examples/kubernetes/
