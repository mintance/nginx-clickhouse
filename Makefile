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

validate-manifests:
	@for f in examples/kubernetes/*.yaml; do \
		echo "validating $$f"; \
		python3 -c "import yaml, sys; list(yaml.safe_load_all(open(sys.argv[1])))" "$$f" || exit 1; \
	done
	@echo "all manifests valid"
