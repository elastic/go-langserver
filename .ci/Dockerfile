# $ docker build --fild Dockerfile --tag elastic/go-langserver.

ARG VERSION=1.12-alpine
FROM golang:${VERSION}

ENV CGO_ENABLED=0
ENV GO111MODULE=off

RUN apk add --no-cache --quiet make curl git jq unzip tree && \
    apk add bash && \
    go get golang.org/x/sync/errgroup

VOLUME /go/src/golang.org/x/tools
WORKDIR /go/src/golang.org/x/tools