FROM golang:1.24 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    make \
    git \
    protobuf-compiler \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

COPY . .

RUN make build

FROM debian:12-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    procps \
    && rm -rf /var/lib/apt/lists/*

RUN curl -s https://packagecloud.io/install/repositories/fdio/release/script.deb.sh | bash

ARG VPP_INSTALL_SKIP_SYSCTL=true

RUN apt-get update && apt-get install -y --no-install-recommends \
    numactl \
    vpp=25.10-release \
    vpp-plugin-core=25.10-release \
    libvppinfra=25.10-release \
    && rm -rf /var/lib/apt/lists/*

RUN curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg > /dev/null \
    && echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr $(lsb_release -s -c) frr-stable" | tee /etc/apt/sources.list.d/frr.list

RUN apt-get update && apt-get install -y --no-install-recommends \
    frr \
    frr-pythontools \
    && rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y --no-install-recommends \
    iproute2 \
    libnuma1 \
    python3-minimal \
    && rm -rf /var/lib/apt/lists/*

RUN apt-get purge -y curl gnupg lsb-release \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /tmp/* /var/tmp/* /usr/share/doc/* /usr/share/man/*

RUN groupadd -r osvbng

RUN mkdir -p /etc/osvbng /var/log/osvbng /var/lib/osvbng /run/osvbng

COPY test-infra/vpp-plugins/*.so /usr/lib/x86_64-linux-gnu/vpp_plugins/

COPY --from=builder /build/bin/osvbngd /usr/local/bin/osvbngd
COPY --from=builder /build/templates /usr/share/osvbng/templates

COPY docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV VPP_STARTUP_CONF=/etc/vpp/startup.conf

EXPOSE 50050/tcp 9090/tcp 8080/tcp

ENTRYPOINT ["/docker-entrypoint.sh"]
