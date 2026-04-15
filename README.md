# Remote Rumble

Remote Rumble is a browser-driven combat robot fighting platform with

## Development

Backend:

- make dev-backend

Frontend:

- make dev-frontend

The frontend dev server proxies /api and /ws to the backend.

## Bot simulator

Use the simulator to spawn many fake bots that connect over websocket,
receive control packets, and stream telemetry like real hardware.

Run:

- go run ./tools/bot-sim

Options:

- --count 4 (number of bots)
- --prefix test-bot- (generated bot IDs like test-bot-1, test-bot-2)
- --start 1 (starting index)
- --ws ws://127.0.0.1:3015/ws (backend websocket endpoint)
- --verbose (log incoming control and command packets)

Example:

- go run ./tools/bot-sim --count 8 --prefix test-bot- --ws ws://127.0.0.1:3015/ws --verbose

Note: Bot IDs must exist in the bots table (add them in the admin panel) to appear in the website bot list.

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