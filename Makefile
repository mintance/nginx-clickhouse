.PHONY: dependencies
dependencies:
	echo "Installing dependencies"
	glide install

.PHONY: code-quality
code-quality:
	gometalinter --vendor --tests --skip=mock --exclude='_gen.go' --disable=gotype --disable=errcheck --disable=gas --disable=dupl --deadline=1500s --checkstyle --sort=linter ./... > static-analysis.xml

install-helpers:
	echo "Installing GoMetaLinter"
	go get -u github.com/alecthomas/gometalinter
	echo "Installing linters"
	gometalinter --install
	echo "Installing Glide"
	curl https://glide.sh/get | sh

build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nginx-clickhouse .

docker:
	docker build --rm --no-cache=true -t mintance/nginx-clickhouse -f Dockerfile .