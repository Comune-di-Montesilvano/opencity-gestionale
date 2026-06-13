FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.AppVersion=${VERSION}" \
    -o gestionale ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/gestionale /gestionale
COPY --from=builder /app/static /static
COPY --from=builder /app/templates /templates
EXPOSE 8080
ENTRYPOINT ["/gestionale"]
