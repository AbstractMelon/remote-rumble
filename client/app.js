const wsStatusEl = document.getElementById("wsStatus");
const padStatusEl = document.getElementById("padStatus");
const presenceStatusEl = document.getElementById("presenceStatus");
const lastPacketEl = document.getElementById("lastPacket");
const telemetryBoxEl = document.getElementById("telemetryBox");
const serverUrlEl = document.getElementById("serverUrl");
const botIdEl = document.getElementById("botId");
const connectBtnEl = document.getElementById("connectBtn");
const disconnectBtnEl = document.getElementById("disconnectBtn");
const buttonGridEl = document.getElementById("buttonGrid");

const axisElements = {
  leftX: document.getElementById("axis-leftX"),
  leftY: document.getElementById("axis-leftY"),
  rightX: document.getElementById("axis-rightX"),
  rightY: document.getElementById("axis-rightY"),
};

const buttonOrder = ["a", "b", "x", "y", "lb", "rb", "select", "start"];
const buttonChipMap = new Map();

for (const name of buttonOrder) {
  const chip = document.createElement("div");
  chip.className = "button-chip";
  chip.textContent = `${name.toUpperCase()}: 0`;
  buttonGridEl.appendChild(chip);
  buttonChipMap.set(name, chip);
}

let socket = null;
let sendIntervalId = null;
let packetSeq = 0;
let gamepadIndex = null;

function clampAxis(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return 0;
  }

  return Math.max(-1, Math.min(1, value));
}

function round3(value) {
  return Math.round(value * 1000) / 1000;
}

function pickGamepad() {
  if (!navigator.getGamepads) {
    return null;
  }

  const pads = navigator.getGamepads();

  if (gamepadIndex !== null && pads[gamepadIndex]) {
    return pads[gamepadIndex];
  }

  for (const pad of pads) {
    if (pad) {
      gamepadIndex = pad.index;
      return pad;
    }
  }

  gamepadIndex = null;
  return null;
}

function getButtonPressed(buttons, index) {
  return Boolean(buttons[index] && buttons[index].pressed);
}

function readControlState() {
  const pad = pickGamepad();

  if (!pad) {
    setPadStatus(false, "waiting");
    return null;
  }

  setPadStatus(true, pad.id || "connected");

  const axes = {
    leftX: round3(clampAxis(pad.axes[0] || 0)),
    leftY: round3(clampAxis(pad.axes[1] || 0)),
    rightX: round3(clampAxis(pad.axes[2] || 0)),
    rightY: round3(clampAxis(pad.axes[3] || 0)),
  };

  const buttons = {
    a: getButtonPressed(pad.buttons, 0),
    b: getButtonPressed(pad.buttons, 1),
    x: getButtonPressed(pad.buttons, 2),
    y: getButtonPressed(pad.buttons, 3),
    lb: getButtonPressed(pad.buttons, 4),
    rb: getButtonPressed(pad.buttons, 5),
    select: getButtonPressed(pad.buttons, 8),
    start: getButtonPressed(pad.buttons, 9),
  };

  updateMeters(axes);
  updateButtons(buttons);

  return { axes, buttons };
}

function updateMeters(axes) {
  for (const [name, element] of Object.entries(axisElements)) {
    const normalized = (axes[name] + 1) / 2;
    element.style.width = `${Math.round(normalized * 100)}%`;
  }
}

function updateButtons(buttons) {
  for (const [name, element] of buttonChipMap.entries()) {
    const isPressed = Boolean(buttons[name]);
    element.classList.toggle("on", isPressed);
    element.textContent = `${name.toUpperCase()}: ${isPressed ? 1 : 0}`;
  }
}

function setWsStatus(online, text) {
  wsStatusEl.className = `chip ${online ? "on" : "off"}`;
  wsStatusEl.textContent = `WebSocket: ${text}`;
}

function setPadStatus(online, text) {
  padStatusEl.className = `chip ${online ? "on" : "off"}`;
  padStatusEl.textContent = `Gamepad: ${text}`;
}

function setPresence(controllers, bots) {
  presenceStatusEl.textContent = `Bots: ${bots} | Controllers: ${controllers}`;
}

function getBaseWsUrl() {
  const input = serverUrlEl.value.trim();
  if (input) {
    return input;
  }

  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${window.location.host}/ws`;
}

function disconnectSocket() {
  if (sendIntervalId) {
    window.clearInterval(sendIntervalId);
    sendIntervalId = null;
  }

  if (socket) {
    socket.close();
    socket = null;
  }

  connectBtnEl.disabled = false;
  disconnectBtnEl.disabled = true;
  setWsStatus(false, "offline");
}

function connectSocket() {
  let url;

  try {
    url = new URL(getBaseWsUrl());
  } catch (_error) {
    setWsStatus(false, "invalid URL");
    return;
  }

  const botId = botIdEl.value.trim() || "test-bot";
  url.searchParams.set("role", "controller");
  url.searchParams.set("botId", botId);

  socket = new WebSocket(url.toString());
  setWsStatus(false, "connecting");

  socket.addEventListener("open", () => {
    setWsStatus(true, "online");
    connectBtnEl.disabled = true;
    disconnectBtnEl.disabled = false;

    sendIntervalId = window.setInterval(() => {
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        return;
      }

      const state = readControlState();
      if (!state) {
        return;
      }

      const payload = {
        type: "control",
        seq: packetSeq,
        sentAt: Date.now(),
        axes: state.axes,
        buttons: state.buttons,
      };

      packetSeq += 1;
      socket.send(JSON.stringify(payload));

      lastPacketEl.textContent = JSON.stringify(payload, null, 2);
    }, 50);
  });

  socket.addEventListener("close", () => {
    disconnectSocket();
  });

  socket.addEventListener("error", () => {
    setWsStatus(false, "error");
  });

  socket.addEventListener("message", (event) => {
    let data;

    try {
      data = JSON.parse(event.data);
    } catch (_error) {
      return;
    }

    if (data.type === "presence") {
      setPresence(data.controllers ?? 0, data.bots ?? 0);
      return;
    }

    if (data.type === "telemetry") {
      telemetryBoxEl.textContent = JSON.stringify(data, null, 2);
    }
  });
}

window.addEventListener("gamepadconnected", (event) => {
  gamepadIndex = event.gamepad.index;
  setPadStatus(true, event.gamepad.id || "connected");
});

window.addEventListener("gamepaddisconnected", (event) => {
  if (event.gamepad.index === gamepadIndex) {
    gamepadIndex = null;
    setPadStatus(false, "disconnected");
  }
});

connectBtnEl.addEventListener("click", connectSocket);
disconnectBtnEl.addEventListener("click", disconnectSocket);

serverUrlEl.value = getBaseWsUrl();
setPresence(0, 0);
setWsStatus(false, "offline");
setPadStatus(false, "waiting");
