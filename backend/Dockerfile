# base image
FROM golang:1.26.10 AS base

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
# The promo entrypoint lives in the private carriers submodule rather than cmd/, so the
# package path falls back there when cmd/ has no such service.
RUN pkg=./cmd/${SERVICE_NAME}; [ -d "$pkg" ] || pkg=./carriers/cmd/${SERVICE_NAME}; \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o app "$pkg"

# One-off maintenance commands ride in the api image, so they run on the production box with
# its own .env and network (docker compose run --rm api-prod ./tools/...) instead of
# needing an image of their own. The directory exists for every service, empty outside
# the api, because the prod stage copies it unconditionally.
RUN mkdir -p tools && if [ "${SERVICE_NAME}" = "api" ]; then \
        CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tools/rebalance-clients ./cmd/rebalance-clients; \
    fi

# prod image
#
# Pinned rather than :latest, so a rebuild of the same commit gets the same base;
# Dependabot proposes the bumps.
FROM alpine:3.24.3 AS prod

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

# api and bot write nothing to disk (the WhatsApp session lives in Postgres), so
# the process needs no privilege at all: an unprivileged user with no home. A
# fixed uid keeps file ownership predictable should a volume ever be mounted.
RUN adduser -D -H -u 10001 app

WORKDIR /app

COPY --from=builder /app/app ./server
COPY --from=builder /app/tools ./tools

USER app

CMD ["./server"]
# prod image for the promo service
#
# Same builder as api and bot - only the runtime differs, because this one drives a
# browser.
#
# Full Chromium from Debian rather than chromedp's headless-shell image: the headless
# shell is a different build of the browser, and the campaign pages stopped working
# with it on 09/11/2026 (the reason is recorded in backend/carriers). Debian is also
# what makes this possible on the arm64 host - Google ships no Linux arm64 Chrome.
#
# Alpine was the first attempt and it is a trap: its chromium pulls 186 packages,
# including gtk+3, the full ICU data and llvm-libs, and the build sat on llvm-libs for
# twenty minutes without finishing.
#
# Fonts are not optional. The screenshots that go back to the group are the operator's
# proof that a CPF was enrolled, and without these every accented character renders as
# a blank box.
#
# curl is here for the Compose healthcheck alone, so probing the port does not need
# bash's /dev/tcp inside YAML.
FROM debian:trixie-slim AS prod-chrome

RUN apt-get update && apt-get install -y --no-install-recommends \
        chromium \
        tzdata \
        curl \
        ca-certificates \
        fonts-liberation \
        fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

# chromedp would search the PATH on its own, but being explicit means a missing or
# renamed binary fails at startup with a clear message instead of on the first campaign.
ENV CHROME_PATH=/usr/bin/chromium

# Unprivileged, but with a home: Chrome writes its crash and config files under
# $HOME, and its per-run profile under /tmp. It already runs with --no-sandbox
# (enroll.go), which is what lets it start without root or extra capabilities.
RUN useradd -m -u 10001 app

WORKDIR /app

COPY --from=builder /app/app ./server

USER app

CMD ["./server"]
