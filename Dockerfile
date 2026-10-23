# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/gpu-governor ./cmd/gpu-governor

FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY ui ./
RUN npm run build

# Debian rather than scratch: the same image runs the GPU probe container,
# and the nvidia-smi binary the NVIDIA toolkit injects needs glibc.
FROM debian:bookworm-slim
ARG VERSION=dev
LABEL org.opencontainers.image.title="GPU Governor" \
    org.opencontainers.image.description="Pauses containers when the GPU or CPU runs too hot and resumes them once it cools down. No work is lost." \
    org.opencontainers.image.vendor="QuantumNovice" \
    org.opencontainers.image.version="$VERSION" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.source="https://github.com/QuantumNovice/docker-gpu-governor" \
    com.docker.desktop.extension.api.version=">= 0.3.4" \
    com.docker.desktop.extension.icon="https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/icon.svg" \
    com.docker.extension.screenshots='[{"alt":"Dashboard with GPU and CPU temperatures","url":"https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/docs/screenshot.png"}]' \
    com.docker.extension.detailed-description="<p>GPU Governor watches NVIDIA GPU and CPU temperatures. When either one stays above its limit, it pauses the containers doing the work. Once temperatures drop below the resume limit, it unpauses them.</p><p>Pausing freezes the processes in place. Memory, open files and GPU memory are kept, so a training run or render continues from the exact point where it stopped.</p><ul><li>Separate pause and resume limits for GPU and CPU</li><li>Pauses GPU containers, labeled containers, or all containers</li><li>Never resumes containers you paused yourself</li><li>Remembers what it paused across restarts and resumes everything when removed</li></ul>" \
    com.docker.extension.publisher-url="https://github.com/QuantumNovice" \
    com.docker.extension.additional-urls='[{"title":"Source code","url":"https://github.com/QuantumNovice/docker-gpu-governor"},{"title":"Report an issue","url":"https://github.com/QuantumNovice/docker-gpu-governor/issues"},{"title":"License","url":"https://github.com/QuantumNovice/docker-gpu-governor/blob/main/LICENSE"}]' \
    com.docker.extension.categories="utility-tools" \
    com.docker.extension.changelog="<p>First release.</p>"

COPY --from=backend /out/gpu-governor /gpu-governor
COPY metadata.json docker-compose.yaml icon.svg /
COPY --from=ui /ui/build /ui
VOLUME /var/lib/gpu-governor
EXPOSE 9090
ENTRYPOINT ["/gpu-governor"]
CMD ["serve"]
