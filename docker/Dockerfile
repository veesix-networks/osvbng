ARG GOLANG_VERSION=1.24
FROM golang:${GOLANG_VERSION} AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    make \
    git \
    protobuf-compiler \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10 && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# The cache mounts persist the module and build caches across image
# builds, so an incremental source change recompiles only its packages
# instead of the whole daemon.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION=${VERSION} COMMIT=${COMMIT} DATE=${DATE} make build

FROM debian:12-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    procps \
    && rm -rf /var/lib/apt/lists/*

ARG VPP_INSTALL_SKIP_SYSCTL=true
ARG DATAPLANE_VERSION=26.06-release

# Dataplane install, two sources. When docker/dataplane-debs/ holds
# debs (staged by `make docker-local-dataplane` from an osvbng-vpp
# release build), install those: patched VPP core and all osvbng
# plugins from one build. Otherwise fall back to upstream packagecloud
# debs; the osvbng plugins then come from the prebuilt drop below.
COPY docker/dataplane-debs/ /tmp/dataplane-debs/
RUN if ls /tmp/dataplane-debs/*.deb >/dev/null 2>&1; then \
        apt-get update && apt-get install -y --no-install-recommends \
        numactl \
        /tmp/dataplane-debs/vpp_*.deb \
        /tmp/dataplane-debs/vpp-plugin-core_*.deb \
        /tmp/dataplane-debs/vpp-plugin-dpdk_*.deb \
        /tmp/dataplane-debs/libvppinfra_*.deb \
        && rm -rf /var/lib/apt/lists/*; \
    else \
        curl -s https://packagecloud.io/install/repositories/fdio/release/script.deb.sh | bash \
        && apt-get update && apt-get install -y --no-install-recommends \
        numactl \
        vpp=${DATAPLANE_VERSION} \
        vpp-plugin-core=${DATAPLANE_VERSION} \
        vpp-plugin-dpdk=${DATAPLANE_VERSION} \
        libvppinfra=${DATAPLANE_VERSION} \
        && rm -rf /var/lib/apt/lists/*; \
    fi

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

# The prebuilt plugin drop applies only to the upstream-deb path; the
# dataplane-debs path already ships the plugins from the same build as
# the core, and overlaying stale binaries would defeat it.
COPY test-infra/vpp-plugins/ /tmp/stock-plugins/
RUN if ls /tmp/dataplane-debs/*.deb >/dev/null 2>&1; then \
        echo "dataplane debs present, skipping prebuilt plugin drop"; \
    else \
        cp /tmp/stock-plugins/*.so /usr/lib/x86_64-linux-gnu/vpp_plugins/; \
    fi \
    && rm -rf /tmp/stock-plugins /tmp/dataplane-debs

COPY --from=builder /build/bin/osvbngd /usr/local/bin/osvbngd
COPY --from=builder /build/bin/osvbngcli /usr/local/bin/osvbngcli
COPY --from=builder /build/templates /usr/share/osvbng/templates

COPY docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENV VPP_STARTUP_CONF=/etc/vpp/startup.conf

EXPOSE 50050/tcp 9090/tcp 8080/tcp

ENTRYPOINT ["/docker-entrypoint.sh"]
