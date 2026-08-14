# base image
FROM golang:1.26.6 AS base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# dev image
FROM base AS dev

RUN go install github.com/air-verse/air@latest

COPY . .

# this default command is just a placeholder
# docker-compose will overwrite this with the specific .toml file
CMD ["air"]

# test image
FROM base AS test

COPY . .
CMD ["go", "test", "./...", "-v"]

# builder image
FROM base AS builder

# argument to define which service will compile
ARG SERVICE_NAME

COPY . .

# compile the static binary (without C dependencies) to be light and safe
# -ldflags="-s -w" removes debug infos for less size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o app ./cmd/${SERVICE_NAME}

# prod image
FROM alpine:latest AS prod

# The DSN in internal/data/db.go pins TimeZone=America/Sao_Paulo, and a bare
# alpine ships no IANA timezone database - so the driver dies with "unknown
# time zone America/Sao_Paulo" immediately after the migrations apply, and both
# Go services crash-loop. It never showed up in dev, test or CI: all three run
# on Debian-based images that carry tzdata already.
#
# TLS needs nothing here: alpine's base already includes ca-certificates-bundle
# with a populated trust store (checked - 119 certificates), so adding
# ca-certificates would be redundant.
RUN apk add --no-cache tzdata

WORKDIR /root/
RUN mkdir -p /root/data

COPY --from=builder /app/app ./server


CMD ["./server"]