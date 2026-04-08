#include <Arduino.h>
#include <ArduinoJson.h>
#include <WebSocketsClient.h>
#include <WiFi.h>
#include "Secrets.h"

#define LED_PIN 2

const int MOTOR1_PIN = 13;
const int MOTOR2_PIN = 12;
const int MOTOR1_CHANNEL = 0;
const int MOTOR2_CHANNEL = 1;
const int ESC_PWM_FREQUENCY_HZ = 50;
const int ESC_PWM_RESOLUTION_BITS = 16;
const int ESC_PULSE_MIN_US = 1000;
const int ESC_PULSE_NEUTRAL_US = 1500;
const int ESC_PULSE_MAX_US = 2000;
const uint32_t CONTROL_TIMEOUT_MS = 350;
const float STEERING_DEADBAND = 0.06f;
const float THROTTLE_DEADBAND = 0.06f;
const uint32_t ESC_ARM_DURATION_MS = 1500;
const uint32_t CONNECTION_LED_BLINK_MS = 400;

const char *WIFI_SSID = RR_WIFI_SSID;
const char *WIFI_PASSWORD = RR_WIFI_PASSWORD;

// The IP or DNS name of the machine running server/index.js.
const char *SERVER_HOST = RR_SERVER_HOST;
const uint16_t SERVER_PORT = RR_SERVER_PORT;
const char *BOT_ID = RR_BOT_ID;

WebSocketsClient wsClient;
uint32_t lastTelemetryMs = 0;
bool ledState = false;
float lastLeftX = 0.0f;
float lastLeftY = 0.0f;
float throttleCmd = 0.0f;
float steeringCmd = 0.0f;
float motor1Cmd = 0.0f;
float motor2Cmd = 0.0f;
uint32_t lastControlMs = 0;
bool failsafeNeutralApplied = false;
bool escArmMode = false;
uint32_t escArmUntilMs = 0;
bool armButtonWasPressed = false;

void updateConnectionLed() {
  if (wsClient.isConnected()) {
    ledState = true;
  } else {
    ledState = ((millis() / CONNECTION_LED_BLINK_MS) % 2) == 0;
  }

  digitalWrite(LED_PIN, ledState ? HIGH : LOW);
}

float clampUnit(float value) {
  if (value > 1.0f) {
    return 1.0f;
  }

  if (value < -1.0f) {
    return -1.0f;
  }

  return value;
}

float applyDeadband(float value, float deadband) {
  return (fabsf(value) < deadband) ? 0.0f : value;
}

uint32_t pulseWidthUsToDuty(int pulseWidthUs) {
  const uint32_t maxDuty = (1UL << ESC_PWM_RESOLUTION_BITS) - 1;
  const float periodUs = 1000000.0f / static_cast<float>(ESC_PWM_FREQUENCY_HZ);
  float duty = (static_cast<float>(pulseWidthUs) / periodUs) * static_cast<float>(maxDuty);

  if (duty < 0.0f) {
    duty = 0.0f;
  }

  if (duty > static_cast<float>(maxDuty)) {
    duty = static_cast<float>(maxDuty);
  }

  return static_cast<uint32_t>(duty + 0.5f);
}

void writeEscOutput(int channel, float command) {
  float clamped = clampUnit(command);
  int pulseWidthUs = ESC_PULSE_NEUTRAL_US;

  if (clamped >= 0.0f) {
    pulseWidthUs = ESC_PULSE_NEUTRAL_US +
                   static_cast<int>(clamped * static_cast<float>(ESC_PULSE_MAX_US - ESC_PULSE_NEUTRAL_US));
  } else {
    pulseWidthUs = ESC_PULSE_NEUTRAL_US +
                   static_cast<int>(clamped * static_cast<float>(ESC_PULSE_NEUTRAL_US - ESC_PULSE_MIN_US));
  }

  if (pulseWidthUs < ESC_PULSE_MIN_US) {
    pulseWidthUs = ESC_PULSE_MIN_US;
  }
  if (pulseWidthUs > ESC_PULSE_MAX_US) {
    pulseWidthUs = ESC_PULSE_MAX_US;
  }

  ledcWrite(channel, pulseWidthUsToDuty(pulseWidthUs));
}

void applyDriveMix(float throttle, float steering) {
  float mixedMotor1 = throttle + steering;
  float mixedMotor2 = throttle - steering;

  float maxAbs = fmaxf(fabsf(mixedMotor1), fabsf(mixedMotor2));
  if (maxAbs > 1.0f) {
    mixedMotor1 /= maxAbs;
    mixedMotor2 /= maxAbs;
  }

  motor1Cmd = clampUnit(mixedMotor1);
  motor2Cmd = clampUnit(mixedMotor2);

  writeEscOutput(MOTOR1_CHANNEL, motor1Cmd);
  writeEscOutput(MOTOR2_CHANNEL, motor2Cmd);
}

void stopDrive() {
  throttleCmd = 0.0f;
  steeringCmd = 0.0f;
  motor1Cmd = 0.0f;
  motor2Cmd = 0.0f;
  writeEscOutput(MOTOR1_CHANNEL, 0.0f);
  writeEscOutput(MOTOR2_CHANNEL, 0.0f);
}

uint32_t getArmRemainingMs() {
  if (!escArmMode) {
    return 0;
  }

  int32_t remaining = static_cast<int32_t>(escArmUntilMs - millis());
  return (remaining > 0) ? static_cast<uint32_t>(remaining) : 0;
}

void enterEscArmMode(const char *reason) {
  stopDrive();
  escArmMode = true;
  escArmUntilMs = millis() + ESC_ARM_DURATION_MS;
  Serial.printf("ESC arm mode entered (%s): neutral for %lu ms\n",
                reason,
                static_cast<unsigned long>(ESC_ARM_DURATION_MS));
}

void updateEscArmMode() {
  if (!escArmMode) {
    return;
  }

  if (static_cast<int32_t>(millis() - escArmUntilMs) >= 0) {
    escArmMode = false;
    Serial.println("ESC arm mode complete: drive enabled");
  }
}

void setupEscOutputs() {
  ledcSetup(MOTOR1_CHANNEL, ESC_PWM_FREQUENCY_HZ, ESC_PWM_RESOLUTION_BITS);
  ledcSetup(MOTOR2_CHANNEL, ESC_PWM_FREQUENCY_HZ, ESC_PWM_RESOLUTION_BITS);
  ledcAttachPin(MOTOR1_PIN, MOTOR1_CHANNEL);
  ledcAttachPin(MOTOR2_PIN, MOTOR2_CHANNEL);

  enterEscArmMode("boot");
}

void checkWifi() {
  if (WiFi.status() == WL_CONNECTED) {
    return;
  }

  Serial.printf("Connecting to WiFi SSID '%s'...\n", WIFI_SSID);
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

  uint32_t waitStart = millis();
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");

    if (millis() - waitStart > 20000) {
      Serial.println("\nWiFi timeout, retrying...");
      WiFi.disconnect(true);
      delay(1000);
      WiFi.begin(WIFI_SSID, WIFI_PASSWORD);
      waitStart = millis();
    }
  }

  Serial.printf("\nWiFi connected. IP=%s RSSI=%d\n", WiFi.localIP().toString().c_str(), WiFi.RSSI());
}

void applyControl(const JsonDocument &document) {
  JsonObjectConst axes = document["axes"].as<JsonObjectConst>();
  JsonObjectConst buttons = document["buttons"].as<JsonObjectConst>();
  bool armButtonPressed = buttons["start"] | false;

  lastLeftX = axes["leftX"] | 0.0f;
  lastLeftY = axes["leftY"] | 0.0f;
  throttleCmd = applyDeadband(clampUnit(-lastLeftY), THROTTLE_DEADBAND);
  steeringCmd = applyDeadband(clampUnit(lastLeftX), STEERING_DEADBAND);

  if (armButtonPressed && !armButtonWasPressed) {
    enterEscArmMode("controller START button");
  }
  armButtonWasPressed = armButtonPressed;

  lastControlMs = millis();
  failsafeNeutralApplied = false;

  updateConnectionLed();
  if (escArmMode) {
    stopDrive();
  } else {
    applyDriveMix(throttleCmd, steeringCmd);
  }

  int throttlePct = static_cast<int>(throttleCmd * 100.0f);
  int steeringPct = static_cast<int>(steeringCmd * 100.0f);
  int motor1Pct = static_cast<int>(motor1Cmd * 100.0f);
  int motor2Pct = static_cast<int>(motor2Cmd * 100.0f);
  const char *mode = escArmMode ? "ARM" : "DRIVE";
  Serial.printf(
      "control mode=%s throttle=%d steering=%d m1=%d m2=%d led=%s\n",
      mode,
      throttlePct,
      steeringPct,
      motor1Pct,
      motor2Pct,
      ledState ? "ON" : "OFF");
}

void sendTelemetry() {
  if (!wsClient.isConnected()) {
    return;
  }

  StaticJsonDocument<256> doc;
  doc["type"] = "telemetry";
  JsonObject data = doc.createNestedObject("data");
  data["wifiRssi"] = WiFi.RSSI();
  data["led"] = ledState;
  data["leftX"] = lastLeftX;
  data["leftY"] = lastLeftY;
  data["throttle"] = throttleCmd;
  data["steering"] = steeringCmd;
  data["motor1"] = motor1Cmd;
  data["motor2"] = motor2Cmd;
  data["escArmMode"] = escArmMode;
  data["armRemainingMs"] = getArmRemainingMs();

  String payload;
  serializeJson(doc, payload);
  wsClient.sendTXT(payload);
}

void onWebSocketEvent(WStype_t type, uint8_t *payload, size_t length) {
  switch (type) {
    case WStype_DISCONNECTED:
      Serial.println("WebSocket disconnected.");
      updateConnectionLed();
      break;
    case WStype_CONNECTED:
      Serial.printf("WebSocket connected: %s\n", payload);
      updateConnectionLed();
      break;
    case WStype_TEXT: {
      StaticJsonDocument<384> doc;
      DeserializationError error = deserializeJson(doc, payload, length);
      if (error) {
        Serial.printf("JSON parse error: %s\n", error.c_str());
        return;
      }

      const char *messageType = doc["type"] | "";
      if (strcmp(messageType, "control") == 0) {
        applyControl(doc);
      }
      break;
    }
    default:
      break;
  }
}

void connectWebSocket() {
  String path = String("/ws?role=bot&botId=") + BOT_ID;
  Serial.printf("Connecting to WebSocket at ws://%s:%d%s\n", SERVER_HOST, SERVER_PORT, path.c_str());
  wsClient.begin(SERVER_HOST, SERVER_PORT, path.c_str());
  wsClient.setReconnectInterval(3000);
  wsClient.onEvent(onWebSocketEvent);
}

void setup() {
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(LED_PIN, LOW);
  Serial.begin(115200);

  setupEscOutputs();
  checkWifi();
  connectWebSocket();
  updateConnectionLed();
}

void loop() {
  checkWifi();
  wsClient.loop();
  updateConnectionLed();
  updateEscArmMode();

  if (!escArmMode && lastControlMs != 0 && millis() - lastControlMs > CONTROL_TIMEOUT_MS && !failsafeNeutralApplied) {
    stopDrive();
    failsafeNeutralApplied = true;
    Serial.println("control timeout -> drive neutral");
  }

  uint32_t now = millis();
  if (now - lastTelemetryMs >= 1000) {
    sendTelemetry();
    lastTelemetryMs = now;
  }
}