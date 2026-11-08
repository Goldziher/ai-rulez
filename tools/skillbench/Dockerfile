# One image for every scan, locally and in CI: the tools are pinned here and in scanners/uv.lock, and
# scans run from it with no network. Build context: tools/skillbench.
FROM golang:1.27-bookworm@sha256:dc0963f32053e4bc451d24d87e3892d42d01021b61f093e98eb95c0352bb84b0 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
ENV GOWORK=off CGO_ENABLED=0 GOTOOLCHAIN=auto
RUN go build -trimpath -o /skillbench ./cmd/skillbench

FROM python:3.13-slim-bookworm@sha256:a5a7017efc5969e00ae2b91c8eb0631049a569652aa4ffc4501769049ae45c72
ARG AI_RULEZ_VERSION
ARG UV_VERSION=0.13.7
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends ca-certificates curl git >/dev/null
RUN pip install --quiet --disable-pip-version-check "uv==${UV_VERSION}"

WORKDIR /opt/scanners
COPY scanners/pyproject.toml scanners/uv.lock ./
ENV UV_PROJECT_ENVIRONMENT=/opt/scanners/.venv
RUN uv sync --frozen --no-dev --no-install-project --quiet

# The released ai-rulez, verified against the release's checksums. A plain go build links a
# machine-local liter-llm, so it is not what users run.
WORKDIR /tmp/ai-rulez
RUN set -eu; \
    arch="$(dpkg --print-architecture)"; \
    base="https://github.com/Goldziher/ai-rulez/releases/download/v${AI_RULEZ_VERSION}"; \
    name="ai-rulez_${AI_RULEZ_VERSION}_linux_${arch}.tar.gz"; \
    curl -fsSLO "${base}/${name}"; curl -fsSLO "${base}/checksums.txt"; \
    grep " ${name}\$" checksums.txt | sha256sum -c -; \
    tar -xzf "${name}"; install -m 0755 "$(find . -type f -name ai-rulez | head -1)" /usr/local/bin/ai-rulez
WORKDIR /
RUN rm -rf /tmp/ai-rulez
COPY --from=build /skillbench /usr/local/bin/skillbench

ENV PATH=/opt/scanners/.venv/bin:/usr/local/bin:/usr/bin:/bin
USER 65534:65534
ENTRYPOINT ["skillbench"]
