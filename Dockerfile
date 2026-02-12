# base image
FROM golang:1.25.7 AS base

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

WORKDIR /root/
RUN mkdir -p /root/data

COPY --from=builder /app/app ./server


CMD ["./server"]