ARG GolangVersion=1.17-alpine
# build stage
FROM golang:${GolangVersion} AS build-env

WORKDIR /go/src/github.com/mintance/nginx-clickhouse

ADD . /go/src/github.com/mintance/nginx-clickhouse

RUN apk update && apk add make g++ git curl
RUN cd /go/src/github.com/mintance/nginx-clickhouse && go get .
RUN cd /go/src/github.com/mintance/nginx-clickhouse && make build

# final stage
FROM scratch

COPY --from=build-env /go/src/github.com/mintance/nginx-clickhouse/nginx-clickhouse /
CMD [ "/nginx-clickhouse" ]
