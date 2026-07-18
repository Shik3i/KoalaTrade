# Single-image build: the Go backend serves the built SPA from the same origin.
#
# Stage 1 builds the frontend, stage 2 embeds that output into the Go binary,
# stage 3 is the minimal runtime.

FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Replace the committed placeholder shell with the real Vite build so it gets
# embedded (//go:embed all:dist) into the binary.
RUN rm -rf ./internal/web/dist
COPY --from=frontend /src/frontend/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/koalatrade ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
	&& addgroup -S koala \
	&& adduser -S -G koala -h /app koala \
	&& mkdir -p /data \
	&& chown -R koala:koala /data
COPY --from=backend /out/koalatrade /usr/local/bin/koalatrade
USER koala
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/koalatrade"]
