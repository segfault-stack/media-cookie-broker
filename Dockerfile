FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/cookie-broker ./cmd/cookie-broker && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/cookie-sync ./cmd/cookie-sync && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/brokerctl ./cmd/brokerctl

FROM alpine:3.22
LABEL org.opencontainers.image.title="media-cookie-broker" \
      org.opencontainers.image.version="0.1.0-preview" \
      org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates && addgroup -S broker && adduser -S -G broker broker \
    && mkdir /data && chown broker:broker /data
COPY --from=build /out/* /usr/local/bin/
USER broker
ENTRYPOINT ["cookie-broker"]
