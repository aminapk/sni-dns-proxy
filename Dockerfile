FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sni-dns-proxy .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/sni-dns-proxy /usr/local/bin/sni-dns-proxy
ENV DATABASE_PATH=/data/proxy.db
VOLUME /data
EXPOSE 53/udp 53/tcp 80/tcp 443/tcp 8080/tcp
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/sni-dns-proxy", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/sni-dns-proxy"]
