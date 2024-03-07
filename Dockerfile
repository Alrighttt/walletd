FROM docker.io/library/golang:1.21 AS builder

WORKDIR /walletd

COPY . .

# Enable CGO for sqlite3 support
ENV CGO_ENABLED=1 

RUN go build -o bin/ -tags='netgo timetzdata' -trimpath -a -ldflags '-s -w -linkmode external -extldflags "-static"'  ./cmd/walletd

FROM docker.io/library/alpine:3

ENV PUID=0
ENV PGID=0

ENV WALLETD_API_PASSWORD=password

# copy binary and prepare data dir.
COPY --from=builder /walletd/bin/* /usr/bin/
VOLUME [ "/data" ]

# API port
EXPOSE 9980/tcp
# RPC port
EXPOSE 9981/tcp

USER ${PUID}:${PGID}

ENTRYPOINT [ "walletd", "-network=komodo","--dir", "/data", "--http", ":9980" ]