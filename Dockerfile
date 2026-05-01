# syntax=docker/dockerfile:1.7

FROM node:24-alpine@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY web/ ./
RUN npm run build

FROM golang:1.26.2@sha256:b54cbf583d390341599d7bcbc062425c081105cc5ef6d170ced98ef9d047c716 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
ARG VERSION=""
ARG COMMIT=""
ARG DIRTY=""
# Resolve build metadata from the in-context .git when not pinned by --build-arg.
# Requires BUILDKIT_CONTEXT_KEEP_GIT_DIR=1 from the caller (set in docker-bake.hcl).
RUN set -eu; \
    VERSION="${VERSION:-$(git -C /src describe --tags --always 2>/dev/null || echo dev)}"; \
    COMMIT="${COMMIT:-$(git -C /src rev-parse HEAD 2>/dev/null || echo none)}"; \
    DIRTY="${DIRTY:-$(if [ -n "$(git -C /src status --porcelain 2>/dev/null)" ]; then echo true; else echo false; fi)}"; \
    PKG=github.com/jmelahman/kanban/cmd/server; \
    CGO_ENABLED=0 GOOS=linux go build -tags embed -trimpath \
      -ldflags="-s -w -X ${PKG}.version=${VERSION} -X ${PKG}.commit=${COMMIT} -X ${PKG}.dirty=${DIRTY}" \
      -o /out/kanban .

FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
RUN apk add --no-cache git ca-certificates
COPY --from=go /out/kanban /kanban
EXPOSE 7474
ENTRYPOINT ["/kanban", "serve"]
