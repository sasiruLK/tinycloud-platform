# Multi-stage cross-compilation build (for local development)
# Stage 1: Build Go binary for target platform
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o api ./cmd/api
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o provider-server ./cmd/provider-server
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o conformance ./cmd/conformance

# Stage 2: Copy binary to minimal runtime image
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /app/api /api
COPY --from=builder /app/provider-server /provider-server
COPY --from=builder /app/conformance /conformance

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/api"]
