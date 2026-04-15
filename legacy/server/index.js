const path = require("path");
const http = require("http");
const express = require("express");
const { WebSocketServer } = require("ws");

const HOST = process.env.HOST || "0.0.0.0";
const PORT = Number(process.env.PORT || 8080);

const app = express();
const clientDir = path.resolve(__dirname, "..", "client");

app.use(express.static(clientDir));

app.get("/health", (_req, res) => {
  res.json({ ok: true, uptimeSec: Math.round(process.uptime()) });
});

const httpServer = http.createServer(app);
const wsServer = new WebSocketServer({ server: httpServer, path: "/ws" });

const sessions = new Map();

function getSession(botId) {
  if (!sessions.has(botId)) {
    sessions.set(botId, { controllers: new Set(), bots: new Set() });
  }

  return sessions.get(botId);
}

function safeSend(socket, payload) {
  if (socket.readyState === 1) {
    socket.send(payload);
  }
}

function notifyPresence(botId) {
  const session = sessions.get(botId);
  if (!session) {
    return;
  }

  const message = JSON.stringify({
    type: "presence",
    botId,
    controllers: session.controllers.size,
    bots: session.bots.size,
  });

  for (const socket of session.controllers) {
    safeSend(socket, message);
  }

  for (const socket of session.bots) {
    safeSend(socket, message);
  }
}

function cleanupSession(botId) {
  const session = sessions.get(botId);
  if (!session) {
    return;
  }

  if (session.controllers.size === 0 && session.bots.size === 0) {
    sessions.delete(botId);
  }
}

wsServer.on("connection", (socket, req) => {
  socket.isAlive = true;
  socket.on("pong", () => {
    socket.isAlive = true;
  });

  const requestUrl = new URL(req.url, `http://${req.headers.host}`);
  const role = requestUrl.searchParams.get("role");
  const botId = requestUrl.searchParams.get("botId");

  if (!botId || (role !== "controller" && role !== "bot")) {
    socket.close(1008, "Expected role and botId query parameters.");
    return;
  }

  const session = getSession(botId);
  const group = role === "controller" ? session.controllers : session.bots;
  group.add(socket);

  safeSend(
    socket,
    JSON.stringify({ type: "welcome", role, botId, serverTime: Date.now() })
  );
  notifyPresence(botId);

  console.log(
    `[connect] role=${role} botId=${botId} controllers=${session.controllers.size} bots=${session.bots.size}`
  );

  socket.on("message", (rawMessage) => {
    let message;

    try {
      message = JSON.parse(rawMessage.toString());
    } catch (_error) {
      safeSend(socket, JSON.stringify({ type: "error", message: "Invalid JSON" }));
      return;
    }

    if (role === "controller" && message.type === "control") {
      const outbound = JSON.stringify({
        type: "control",
        botId,
        seq: message.seq ?? null,
        sentAt: message.sentAt ?? Date.now(),
        axes: message.axes ?? {},
        buttons: message.buttons ?? {},
      });

      for (const botSocket of session.bots) {
        safeSend(botSocket, outbound);
      }

      return;
    }

    if (role === "bot" && message.type === "telemetry") {
      const outbound = JSON.stringify({
        type: "telemetry",
        botId,
        sentAt: Date.now(),
        data: message.data ?? {},
      });

      for (const controllerSocket of session.controllers) {
        safeSend(controllerSocket, outbound);
      }
    }
  });

  socket.on("close", () => {
    group.delete(socket);
    notifyPresence(botId);
    cleanupSession(botId);

    const updated = sessions.get(botId);
    const controllers = updated ? updated.controllers.size : 0;
    const bots = updated ? updated.bots.size : 0;
    console.log(`[disconnect] role=${role} botId=${botId} controllers=${controllers} bots=${bots}`);
  });

  socket.on("error", (error) => {
    console.error(`[socket-error] role=${role} botId=${botId}`, error.message);
  });
});

const heartbeatTimer = setInterval(() => {
  for (const socket of wsServer.clients) {
    if (socket.isAlive === false) {
      socket.terminate();
      continue;
    }

    socket.isAlive = false;
    socket.ping();
  }
}, 30000);

wsServer.on("close", () => {
  clearInterval(heartbeatTimer);
});

httpServer.listen(PORT, HOST, () => {
  console.log(`Listening on http://${HOST}:${PORT}`);
  console.log(`WebSocket endpoint: ws://${HOST}:${PORT}/ws`);
});
