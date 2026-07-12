# syntax=docker/dockerfile:1.7

FROM node:26-alpine AS web
WORKDIR /src
COPY web/package.json web/package-lock.json* ./web/
RUN cd web && npm install --no-audit --no-fund
COPY web ./web
WORKDIR /src/web
RUN npm run build

FROM golang:1.26.5-alpine AS go
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o /out/dodo ./cmd/dodo

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S dodo && adduser -S -G dodo dodo && \
    mkdir -p /data && chown dodo:dodo /data
COPY --from=go /out/dodo /usr/local/bin/dodo
ENV DODO_DATABASE_PATH=/data/dodo.sqlite \
    DODO_LISTEN=:8080
EXPOSE 8080/tcp
VOLUME ["/data"]
HEALTHCHECK CMD wget -qO- localhost:8080/healthz || exit 1
USER dodo
LABEL org.opencontainers.image.source="https://github.com/mtzanidakis/dodo"
LABEL org.opencontainers.image.licenses="MIT"
ENTRYPOINT ["dodo"]
CMD ["serve"]