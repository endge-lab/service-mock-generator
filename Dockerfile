# syntax=docker/dockerfile:1.7

ARG BASE_BUILDER_IMAGE=golang:1.26.1-alpine
ARG BASE_RUNTIME_IMAGE=alpine:3.21
FROM ${BASE_BUILDER_IMAGE} AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -buildvcs=false -o /out/service-template-go ./cmd/main.go

FROM ${BASE_RUNTIME_IMAGE}

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app \
  && apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/service-template-go /app/service-template-go
COPY migrations /app/migrations
COPY docs /app/docs

USER app

EXPOSE 8080

ENTRYPOINT ["/app/service-template-go"]
