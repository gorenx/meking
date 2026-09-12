# syntax=docker/dockerfile:1.7

FROM golang:1.26-bookworm AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/meking ./cmd/meking

FROM python:3.12-slim-bookworm AS analysis-builder

COPY --from=ghcr.io/astral-sh/uv:0.11.30 /uv /uvx /bin/
ENV UV_PYTHON_DOWNLOADS=0 \
    UV_COMPILE_BYTECODE=1
WORKDIR /opt/meking/analysis-sidecar
COPY analysis-sidecar/pyproject.toml analysis-sidecar/uv.lock ./
COPY analysis-sidecar/src ./src
COPY analysis-sidecar/scripts ./scripts
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev --no-editable && \
    .venv/bin/python scripts/install_resources.py --destination .venv/nltk_data

FROM python:3.12-slim-bookworm

RUN apt-get update && \
    apt-get install --no-install-recommends -y ca-certificates libgomp1 && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd --gid 10001 meking && \
    useradd --uid 10001 --gid meking --no-create-home --home-dir /nonexistent meking
COPY --from=go-builder /out/meking /usr/local/bin/meking
COPY --from=analysis-builder /opt/meking/analysis-sidecar/.venv /opt/meking/analysis-sidecar/.venv

ENV PATH="/opt/meking/analysis-sidecar/.venv/bin:${PATH}" \
    PYTHONDONTWRITEBYTECODE=1
WORKDIR /project
USER meking

EXPOSE 8080
ENTRYPOINT ["meking"]
CMD ["start", "--root", "/project", "--address", "0.0.0.0", "--port", "8080"]
