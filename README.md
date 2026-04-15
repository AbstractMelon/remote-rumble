# Remote Rumble

Remote Rumble is a browser-driven combat robot fighting platform with

## Development

Backend:

- make dev-backend

Frontend:

- make dev-frontend

The frontend dev server proxies /api and /ws to the backend.

## Production build

- make build

This will:

1. Build frontend static assets into frontend/build
2. Copy assets into backend/web for embedding
3. Build the single Go binary at remote-rumble

Run it with:

- ./remote-rumble

## Docker compose

- make docker-up

or:

- docker compose up -d

Services:

- app on port 3015
- mediamtx on ports 8889 (WebRTC/WHIP/WHEP) and 8888 (optional HLS)