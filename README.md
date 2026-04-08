# Remote Rumble

Remote rumble is an OTA combat robot fighting application

1. Browser reads gamepad input.
2. Browser sends control packets to a relay server over WebSocket.
3. ESP32 bot connects to the relay over WebSocket.
4. ESP32 receives packets and applies them (LED + serial output).

## Components

- `client/`: browser UI that reads the Gamepad API and streams controls.
- `server/`: Node.js relay that pairs controller and bot by `botId`.
- `bot/`: PlatformIO firmware that connects as `role=bot` and listens for control messages.

## Quick Start (Local Network)

### 1) Start relay server

```bash
cd server
npm install
npm run dev
```

The web app is served from the same process at `http://localhost:8080`.

### 2) Open controller web app

Open `http://localhost:8080` in a desktop browser, plug in a controller, and click Connect.

Use the default Bot ID (`test-bot`) unless you changed it in firmware.

### 3) Configure and upload ESP32 firmware

Create a private config file from the example template:

```bash
cp bot/include/secrets.example.h bot/include/secrets.h
```

Then edit `bot/include/secrets.h` with your private values:

- `RR_WIFI_SSID`
- `RR_WIFI_PASSWORD`
- `RR_SERVER_HOST` (IP/DNS of machine running `server/index.js`)
- `RR_SERVER_PORT`
- `RR_BOT_ID` (must match browser Bot ID)

`bot/include/secrets.h` is git-ignored, so your WiFi password is not committed.

Then upload from the `bot` folder:

```bash
pio run -t upload
pio device monitor
```

### 4) Validate behavior

- If you press `A` on the controller, the ESP32 onboard LED should turn on.
- Move left stick -> serial monitor should print throttle/steering plus mixed motor outputs.
- Motor 1 ESC signal is on GPIO13 and Motor 2 ESC signal is on GPIO12.
- Mixing is done in firmware (no ESC-side mixing):
  - `motor1 = throttle + steering`
  - `motor2 = throttle - steering`
- Press `START` on the controller at any time to re-enter ESC arm mode (neutral output for 1.5s).
- If control packets stop for ~350ms, both outputs are forced back to neutral.
- Web app should show bot telemetry and presence counts.

## Message Shape

Controller to relay:

```json
{
  "type": "control",
  "seq": 1,
  "sentAt": 1700000000000,
  "axes": { "leftX": 0.1, "leftY": -0.7, "rightX": 0.0, "rightY": 0.0 },
  "buttons": { "a": true, "b": false, "x": false, "y": false, "lb": false, "rb": false, "select": false, "start": false }
}
```

Bot to relay (telemetry):

```json
{
  "type": "telemetry",
  "data": { "wifiRssi": -55, "led": true, "leftX": 0.1, "leftY": -0.7 }
}
```
