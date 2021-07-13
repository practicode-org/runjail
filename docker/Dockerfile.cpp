# syntax=docker/dockerfile:1
FROM alpine:latest as build-runjail

RUN apk add --no-cache make go git wget

ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH

WORKDIR /build/
COPY . .

RUN make

##################
FROM alpine:latest

RUN apk add --no-cache llvm

COPY --from=build-runjail /build/bin/main /runjail
COPY ./examples/cpp-rules.json /run/cpp-rules.json

EXPOSE 1556
RUN mkdir /tmp/sources/ && chmod ugo+rwx /tmp/sources/
ENTRYPOINT /runjail -rules /run/cpp-rules.json
