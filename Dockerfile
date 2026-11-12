# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.23-alpine AS build
WORKDIR /src
# Cache modules (none beyond stdlib, but keeps the layer stable).
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/android-tv-relay ./cmd/android-tv-relay

# ---- runtime ----
FROM gcr.io/distroless/static:nonroot
LABEL org.opencontainers.image.source="https://github.com/shiham-dev/android-tv-relay" \
      org.opencontainers.image.description="Control Android/Google TVs over ADB via HTTP + TCP" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/android-tv-relay /usr/local/bin/android-tv-relay
# /data holds the persisted ADB key; mount a volume or Secret here.
VOLUME ["/data"]
EXPOSE 8080 8700
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/android-tv-relay"]
