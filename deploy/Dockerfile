# syntax=docker/dockerfile:1
# Release images. The release workflow builds them from the binaries
# tools/release-server.sh made, so they match the release bundles:
#   docker buildx build -f deploy/Dockerfile --target collector --platform linux/amd64,linux/arm64 .
# Nothing runs at build time, so other platforms need no emulation.
# The root Dockerfile builds from source for development instead.

FROM alpine:3.22 AS collector
ARG TARGETARCH
LABEL org.opencontainers.image.source="https://github.com/dhamith93/SyMon" \
      org.opencontainers.image.description="SyMon collector" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY --chmod=755 dist/linux_${TARGETARCH}/collector ./collector
USER 10001:10001
EXPOSE 9000
CMD ["./collector"]

FROM alpine:3.22 AS client
ARG TARGETARCH
LABEL org.opencontainers.image.source="https://github.com/dhamith93/SyMon" \
      org.opencontainers.image.description="SyMon dashboard, with the signed agent builds new hosts download" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY --chmod=755 dist/linux_${TARGETARCH}/client ./client
COPY dist/agents/ ./downloads/
USER 10001:10001
EXPOSE 8080
CMD ["./client"]
