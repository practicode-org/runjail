# syntax=docker/dockerfile:1
FROM ubuntu:18.04 as build-runner

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y make golang-1.13 git

ENV PATH /usr/lib/go-1.13/bin:$PATH

WORKDIR /build/
COPY . .

RUN make

##################
FROM ubuntu:18.04 as build-nsjail

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
  make \
  git \
  g++ \
  flex \
  bison \
  autoconf \
  libprotobuf-dev \
  libnl-route-3-dev \
  libtool \
  make \
  pkg-config \
  protobuf-compiler

RUN git clone https://github.com/google/nsjail
COPY ./docker/nsjail-1.patch /nsjail/
WORKDIR /nsjail
# commit where the following patch can be applied
RUN git checkout 2e9fd0e2e
RUN git apply nsjail-1.patch
RUN make

##################
FROM ubuntu:18.04

ENV DEBIAN_FRONTEND=noninteractive

COPY --from=build-runner /build/bin/main /runner
COPY --from=build-nsjail /nsjail/nsjail /usr/bin/nsjail
COPY ./rules /run/rules

RUN apt-get update && apt-get install -y wget lsb-release software-properties-common libprotobuf10 libnl-route-3-200
RUN bash -c "$(wget -O - https://apt.llvm.org/llvm.sh)"

EXPOSE 1556
RUN mkdir /tmp/sources/ && chmod ugo+rwx /tmp/sources/
RUN mkdir /tmp/out
ENTRYPOINT /runner -rules-dir /run/rules
