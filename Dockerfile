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
    org.opencontainers.image.description="Pause containers when your NVIDIA GPU or CPU overheats and resume them automatically when it cools down. No work is lost." \
    org.opencontainers.image.vendor="QuantumNovice" \
    org.opencontainers.image.authors="QuantumNovice" \
    org.opencontainers.image.version="$VERSION" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://hub.docker.com/r/hasisyed/gpu-governor" \
    org.opencontainers.image.documentation="https://github.com/QuantumNovice/docker-gpu-governor#readme" \
    org.opencontainers.image.source="https://github.com/QuantumNovice/docker-gpu-governor" \
    com.docker.desktop.extension.api.version=">= 0.3.4" \
    com.docker.desktop.extension.icon="https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/icon.svg" \
    com.docker.extension.screenshots='[{"alt":"GPU over its temperature limit with two containers paused for cooling","url":"https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/docs/screenshot.png"},{"alt":"Normal temperatures with GPU containers running","url":"https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/docs/screenshot-normal.png"},{"alt":"Pause and resume temperature limits for GPU and CPU","url":"https://raw.githubusercontent.com/QuantumNovice/docker-gpu-governor/main/docs/screenshot-settings.png"}]' \
    com.docker.extension.detailed-description="<p><b>Protect your GPU and CPU from overheating during long Docker workloads.</b> GPU Governor monitors NVIDIA GPU temperature and CPU temperature. When either one stays above its limit, it pauses the containers doing the work. Once temperatures drop below the resume limit, it resumes them automatically.</p><p>Pausing freezes the processes in place. RAM, open files and GPU memory are kept, so a PyTorch training run, Stable Diffusion render, Ollama model or batch job continues from the exact point where it stopped. Nothing is restarted and no work is lost.</p><p>Built for gaming laptops, compact PCs and home labs that run AI, machine learning and CUDA workloads in Docker, where thermal throttling and overheating are common.</p><ul><li>Live GPU and CPU temperature dashboard</li><li>Separate pause and resume limits so containers do not flap</li><li>Ignores short temperature spikes</li><li>Pauses GPU containers, labeled containers, or all containers</li><li>Never resumes containers you paused yourself</li><li>Remembers what it paused across restarts and resumes everything when removed</li><li>HTTP API for scripts and monitoring</li></ul><p>Requires an NVIDIA GPU with Docker Desktop on Windows (WSL 2) or Linux with the NVIDIA Container Toolkit.</p>" \
    com.docker.extension.publisher-url="https://github.com/QuantumNovice" \
    com.docker.extension.additional-urls='[{"title":"Documentation","url":"https://github.com/QuantumNovice/docker-gpu-governor#readme"},{"title":"Source code","url":"https://github.com/QuantumNovice/docker-gpu-governor"},{"title":"Docker Hub","url":"https://hub.docker.com/r/hasisyed/gpu-governor"},{"title":"Report an issue","url":"https://github.com/QuantumNovice/docker-gpu-governor/issues"},{"title":"License","url":"https://github.com/QuantumNovice/docker-gpu-governor/blob/main/LICENSE"}]' \
    com.docker.extension.categories="utility-tools" \
    com.docker.extension.changelog="<ul><li>Clearer listing text and three screenshots</li><li>Image metadata links to documentation and Docker Hub</li></ul>"

COPY --from=backend /out/gpu-governor /gpu-governor
COPY metadata.json docker-compose.yaml icon.svg /
COPY --from=ui /ui/build /ui
VOLUME /var/lib/gpu-governor
EXPOSE 9090
ENTRYPOINT ["/gpu-governor"]
CMD ["serve"]
