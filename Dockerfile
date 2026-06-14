FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.AppVersion=${VERSION}" \
    -o gestionale ./cmd/server
RUN mkdir /data

FROM gcr.io/distroless/static:nonroot
COPY --from=builder --chown=65532:65532 /app/gestionale /gestionale
COPY --from=builder --chown=65532:65532 /app/static /static
COPY --from=builder --chown=65532:65532 /app/templates /templates
COPY --from=builder --chown=65532:65532 /data /data
EXPOSE 8080
ENTRYPOINT ["/gestionale"]
