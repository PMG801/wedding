# syntax=docker/dockerfile:1
ARG GO_VERSION=1.23
ARG NODE_VERSION=22
ARG CADDY_VERSION=2

# -----------------------
# Stage: frontend-build
# -----------------------
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# -----------------------
# Stage: backend-build
# -----------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS backend-build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app/backend
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/boda ./cmd/boda

# -----------------------
# Target: app
# -----------------------
FROM gcr.io/distroless/static-debian12:nonroot AS app
COPY --from=backend-build --chown=nonroot:nonroot /out/boda /usr/local/bin/boda
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/boda"]
CMD ["serve"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/usr/local/bin/boda", "check"]

# -----------------------
# Target: web
# -----------------------
FROM caddy:${CADDY_VERSION}-alpine AS web
COPY --from=frontend-build /app/frontend/dist /srv
COPY deploy/caddy/Caddyfile /etc/caddy/Caddyfile
