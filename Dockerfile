FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/cookie-broker ./cmd/cookie-broker && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/cookie-sync ./cmd/cookie-sync && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/brokerctl ./cmd/brokerctl

FROM alpine:3.22 AS runtime
LABEL org.opencontainers.image.title="media-cookie-broker" \
      org.opencontainers.image.version="0.3.0-preview" \
      org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates && addgroup -S broker && adduser -S -G broker broker
USER broker

FROM runtime AS cookie-sync
LABEL org.opencontainers.image.title="media-cookie-broker-cookie-sync"
COPY --from=build /out/cookie-sync /usr/local/bin/cookie-sync
ENTRYPOINT ["cookie-sync"]

FROM runtime AS broker
USER root
RUN mkdir /data && chown broker:broker /data
COPY --from=build /out/cookie-broker /out/brokerctl /usr/local/bin/
USER broker
ENTRYPOINT ["cookie-broker"]
