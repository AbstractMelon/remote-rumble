package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	controlTimeout   = 350 * time.Millisecond
	steeringDeadband = 0.06
	throttleDeadband = 0.06
	escArmDuration   = 1500 * time.Millisecond
)

type config struct {
	wsBaseURL         string
	count             int
	prefix            string
	startIndex        int
	telemetryInterval time.Duration
	reconnectInterval time.Duration
	pingInterval      time.Duration
	verbose           bool
	seed              int64
}

type botState struct {
	mu sync.Mutex

	rng *rand.Rand

	enabled  bool
	led      bool
	wifiRssi int

	lastLeftX float64
	lastLeftY float64

	throttle float64
	steering float64
	motor1   float64
	motor2   float64

	lastControl            time.Time
	failsafeNeutralApplied bool

	escArmMode          bool
	armUntil            time.Time
	armButtonWasPressed bool
}

func newBotState(seed int64) *botState {
	r := rand.New(rand.NewSource(seed))
	return &botState{
		rng:      r,
		enabled:  true,
		led:      true,
		wifiRssi: -55 - r.Intn(15),
	}
}

func (s *botState) applyControl(leftX, leftY float64, startButton bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastLeftX = clampUnit(leftX)
	s.lastLeftY = clampUnit(leftY)

	if startButton && !s.armButtonWasPressed {
		s.enterEscArmModeLocked()
	}
	s.armButtonWasPressed = startButton

	s.throttle = applyDeadband(clampUnit(-s.lastLeftY), throttleDeadband)
	s.steering = applyDeadband(clampUnit(s.lastLeftX), steeringDeadband)
	s.lastControl = time.Now()
	s.failsafeNeutralApplied = false

	if !s.enabled || s.escArmMode {
		s.stopDriveLocked()
		return
	}

	s.motor1, s.motor2 = applyDriveMix(s.throttle, s.steering)
}

func (s *botState) applyCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch strings.ToLower(strings.TrimSpace(command)) {
	case "disable":
		s.enabled = false
		s.stopDriveLocked()
	case "enable":
		s.enabled = true
	}
}

func (s *botState) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.escArmMode && !now.Before(s.armUntil) {
		s.escArmMode = false
	}

	s.wifiRssi += s.rng.Intn(5) - 2
	if s.wifiRssi < -90 {
		s.wifiRssi = -90
	}
	if s.wifiRssi > -30 {
		s.wifiRssi = -30
	}

	if !s.enabled {
		s.stopDriveLocked()
		return
	}

	if !s.escArmMode && !s.lastControl.IsZero() && now.Sub(s.lastControl) > controlTimeout && !s.failsafeNeutralApplied {
		s.stopDriveLocked()
		s.failsafeNeutralApplied = true
	}
}

func (s *botState) telemetryData() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	remaining := int64(0)
	if s.escArmMode {
		remaining = time.Until(s.armUntil).Milliseconds()
		if remaining < 0 {
			remaining = 0
		}
	}

	return map[string]any{
		"wifiRssi":       s.wifiRssi,
		"led":            s.led,
		"leftX":          roundTo(s.lastLeftX, 3),
		"leftY":          roundTo(s.lastLeftY, 3),
		"throttle":       roundTo(s.throttle, 3),
		"steering":       roundTo(s.steering, 3),
		"motor1":         roundTo(s.motor1, 3),
		"motor2":         roundTo(s.motor2, 3),
		"escArmMode":     s.escArmMode,
		"armRemainingMs": remaining,
	}
}

func (s *botState) enterEscArmModeLocked() {
	s.stopDriveLocked()
	s.escArmMode = true
	s.armUntil = time.Now().Add(escArmDuration)
}

func (s *botState) stopDriveLocked() {
	s.throttle = 0
	s.steering = 0
	s.motor1 = 0
	s.motor2 = 0
}

func runBotLoop(ctx context.Context, cfg config, botID string, index int) {
	for {
		if ctx.Err() != nil {
			return
		}

		err := runBotConnection(ctx, cfg, botID, index)
		if ctx.Err() != nil {
			return
		}

		if err != nil {
			log.Printf("[%s] disconnected: %v", botID, err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.reconnectInterval):
		}
	}
}

func runBotConnection(ctx context.Context, cfg config, botID string, index int) error {
	botURL, err := buildBotURL(cfg.wsBaseURL, botID)
	if err != nil {
		return err
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, botURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetReadLimit(256 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	})

	state := newBotState(cfg.seed + int64(index*101))
	var writeMu sync.Mutex

	log.Printf("[%s] connected to %s", botID, botURL)

	errCh := make(chan error, 2)
	go readLoop(conn, &writeMu, state, botID, cfg.verbose, errCh)
	go writeLoop(ctx, conn, &writeMu, state, cfg, errCh)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func readLoop(conn *websocket.Conn, writeMu *sync.Mutex, state *botState, botID string, verbose bool, errCh chan<- error) {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			sendErr(errCh, err)
			return
		}

		var msg map[string]any
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}

		switch asString(msg["type"]) {
		case "control":
			axes, _ := msg["axes"].(map[string]any)
			buttons, _ := msg["buttons"].(map[string]any)
			leftX := asFloat(axes["leftX"])
			leftY := asFloat(axes["leftY"])
			startButton := asBool(buttons["start"])
			state.applyControl(leftX, leftY, startButton)
			if verbose {
				log.Printf("[%s] control leftX=%.2f leftY=%.2f start=%t", botID, leftX, leftY, startButton)
			}
		case "command":
			command := asString(msg["command"])
			state.applyCommand(command)
			if verbose {
				log.Printf("[%s] command=%s", botID, command)
			}
		case "ping":
			_ = writeJSON(conn, writeMu, map[string]any{"type": "pong", "t": msg["t"]}, errCh)
		}
	}
}

func writeLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, state *botState, cfg config, errCh chan<- error) {
	telemetryTicker := time.NewTicker(cfg.telemetryInterval)
	pingTicker := time.NewTicker(cfg.pingInterval)
	defer telemetryTicker.Stop()
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-telemetryTicker.C:
			state.tick()
			msg := map[string]any{"type": "telemetry", "data": state.telemetryData()}
			if err := writeJSON(conn, writeMu, msg, errCh); err != nil {
				return
			}
		case <-pingTicker.C:
			msg := map[string]any{"type": "ping", "t": time.Now().UnixMilli()}
			if err := writeJSON(conn, writeMu, msg, errCh); err != nil {
				return
			}
		}
	}
}

func writeJSON(conn *websocket.Conn, writeMu *sync.Mutex, payload any, errCh chan<- error) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(8 * time.Second)); err != nil {
		sendErr(errCh, err)
		return err
	}
	if err := conn.WriteJSON(payload); err != nil {
		sendErr(errCh, err)
		return err
	}
	return nil
}

func sendErr(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}

func buildBotURL(baseURL, botID string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("ws URL must use ws or wss scheme: %s", baseURL)
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("ws URL is missing host: %s", baseURL)
	}
	if strings.TrimSpace(u.Path) == "" {
		u.Path = "/ws"
	}

	q := u.Query()
	q.Set("role", "bot")
	q.Set("botId", botID)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func clampUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func applyDeadband(v, deadband float64) float64 {
	if math.Abs(v) < deadband {
		return 0
	}
	return v
}

func applyDriveMix(throttle, steering float64) (float64, float64) {
	m1 := throttle + steering
	m2 := throttle - steering
	maxAbs := math.Max(math.Abs(m1), math.Abs(m2))
	if maxAbs > 1 {
		m1 /= maxAbs
		m2 /= maxAbs
	}
	return clampUnit(m1), clampUnit(m2)
}

func roundTo(v float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(v*factor) / factor
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.wsBaseURL, "ws", "ws://127.0.0.1:3015/ws", "base websocket endpoint")
	flag.IntVar(&cfg.count, "count", 4, "number of simulated bots")
	flag.StringVar(&cfg.prefix, "prefix", "sim-bot-", "bot ID prefix")
	flag.IntVar(&cfg.startIndex, "start", 1, "starting index for generated bot IDs")
	flag.DurationVar(&cfg.telemetryInterval, "telemetry", 250*time.Millisecond, "telemetry send interval")
	flag.DurationVar(&cfg.reconnectInterval, "reconnect", 2*time.Second, "delay before reconnecting after disconnect")
	flag.DurationVar(&cfg.pingInterval, "ping", 20*time.Second, "heartbeat ping interval")
	flag.BoolVar(&cfg.verbose, "verbose", false, "log control/command packets")
	flag.Int64Var(&cfg.seed, "seed", time.Now().UnixNano(), "random seed")
	flag.Parse()

	if cfg.count < 1 {
		log.Fatalf("count must be >= 1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting bot simulator: count=%d ws=%s prefix=%s start=%d", cfg.count, cfg.wsBaseURL, cfg.prefix, cfg.startIndex)

	var wg sync.WaitGroup
	for i := 0; i < cfg.count; i++ {
		botID := fmt.Sprintf("%s%d", cfg.prefix, cfg.startIndex+i)
		wg.Add(1)
		go func(id string, idx int) {
			defer wg.Done()
			runBotLoop(ctx, cfg, id, idx)
		}(botID, i)
	}

	<-ctx.Done()
	log.Printf("shutdown requested, waiting for bots to stop")
	wg.Wait()
	log.Printf("bot simulator stopped")
}
