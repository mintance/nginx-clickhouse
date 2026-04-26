# build stage
FROM golang:1.26-alpine AS build-env

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o nginx-clickhouse .

# final stage
FROM scratch

COPY --from=build-env /src/nginx-clickhouse /
CMD [ "/nginx-clickhouse" ]
