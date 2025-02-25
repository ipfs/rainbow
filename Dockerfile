FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.24-bookworm AS builder

LABEL org.opencontainers.image.source=https://github.com/ipfs/rainbow
LABEL org.opencontainers.image.documentation=https://github.com/ipfs/rainbow#docker
LABEL org.opencontainers.image.description="A standalone IPFS Gateway"
LABEL org.opencontainers.image.licenses=MIT+APACHE_2.0


# This builds rainbow

ARG TARGETPLATFORM TARGETOS TARGETARCH

ENV GOPATH="/go"
ENV SRC_PATH="$GOPATH/src/github.com/ipfs/rainbow"
ENV GO111MODULE=on
ENV GOPROXY="https://proxy.golang.org"

COPY go.* $SRC_PATH/
WORKDIR $SRC_PATH
RUN go mod download

COPY . $SRC_PATH
RUN git config --global --add safe.directory /go/src/github.com/ipfs/rainbow

RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o $GOPATH/bin/rainbow

#------------------------------------------------------
FROM debian:bookworm-slim

# Instal binaries for $TARGETARCH
RUN apt-get update && \
  apt-get install --no-install-recommends -y tini ca-certificates curl && \
  rm -rf /var/lib/apt/lists/*

ENV GOPATH="/go"
ENV SRC_PATH="$GOPATH/src/github.com/ipfs/rainbow"
ENV RAINBOW_GATEWAY_PATH="/data/rainbow"

COPY --from=builder $GOPATH/bin/rainbow /usr/local/bin/rainbow
COPY --from=builder $SRC_PATH/docker/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN mkdir -p $RAINBOW_GATEWAY_PATH && \
    useradd -d $RAINBOW_GATEWAY_PATH -u 1000 -G users ipfs && \
    chown ipfs:users $RAINBOW_GATEWAY_PATH
VOLUME $RAINBOW_GATEWAY_PATH
WORKDIR $RAINBOW_GATEWAY_PATH
USER ipfs
ENTRYPOINT ["tini", "--", "/usr/local/bin/entrypoint.sh"]

