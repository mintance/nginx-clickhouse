.PHONY: build docker lint test test-integration test-e2e validate-manifests

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

validate-manifests: # servicemonitor.yaml excluded: requires Prometheus Operator CRDs
	kubectl apply --dry-run=client -f examples/kubernetes/configmap.yaml
	kubectl apply --dry-run=client -f examples/kubernetes/secret.yaml
	kubectl apply --dry-run=client -f examples/kubernetes/sidecar-deployment.yaml
	kubectl apply --dry-run=client -f examples/kubernetes/daemonset.yaml
