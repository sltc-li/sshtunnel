FROM golang:1.16.5 AS builder

ENV GOFLAGS="-mod=vendor"

WORKDIR /src

COPY ./go.mod ./
COPY ./go.sum ./
RUN go mod download

COPY . ./
RUN go mod vendor
RUN go build --tags netgo --ldflags 'extflags=-static' -o sshtunnel ./cmd/tunnel/main.go

FROM alpine:latest
COPY --from=builder /src/sshtunnel /
ENTRYPOINT ["/sshtunnel"]
