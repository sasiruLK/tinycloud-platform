# Multi-stage cross-compilation build
# Stage 1: Build on native platform (buildx handles this)
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o api ./cmd/api

# Stage 2: Copy binary to target platform image
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot

COPY --from=builder /app/api /api

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/api"]
