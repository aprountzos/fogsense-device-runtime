package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fogsense-device-runtime/pkg/device"

	"github.com/eclipse/paho.golang/autopaho"
)

const (
	MaxQueue      = 500
	PostTimeout   = 3 * time.Second
	MaxBackoff    = 30 * time.Second
	FlushInterval = 250 * time.Millisecond
)

type OutboundMessage struct {
	MessageID string                 `json:"messageId"`
	Path      string                 `json:"path"`
	Body      map[string]interface{} `json:"body"`
}

type ClientStats struct {
	Registered bool  `json:"registered"`
	Queued     int   `json:"queued"`
	Dropped    int64 `json:"dropped"`
	Seq        int64 `json:"seq"`
	OutboxPath string `json:"outboxPath,omitempty"`
}

type Client struct {
	mu                  sync.Mutex
	gatewayURL          string
	dev                 *device.Device
	endpoint            string
	token               string
	registerIntervalSec time.Duration
	seq                 int64
	queue               []OutboundMessage
	outboxPath          string
	outboxErr           error
	dropped             int64
	registered          bool
	stopped             bool
	flushFailures       int
	registerFailures    int
	httpClient          *http.Client
	transport           string
	mqttURL             string
	mqttClientID        string
	mqttUsername        string
	mqttPassword        string
	mqttCAFile          string
	mqttClientCertFile  string
	mqttClientKeyFile   string
	mqtt                *autopaho.ConnectionManager
	mqttReady           bool
	mqttAckWaiters      map[string]chan bool
	mqttCancel          context.CancelFunc
	kickChan            chan struct{}
	stopChan            chan struct{}
}

type Config struct {
	GatewayURL          string
	Device              *device.Device
	Endpoint            string
	Token               string
	RegisterIntervalSec int
	Transport           string
	MQTTURL             string
	MQTTClientID        string
	MQTTUsername        string
	MQTTPassword        string
	MQTTCAFile          string
	MQTTClientCertFile  string
	MQTTClientKeyFile   string
	OutboxPath          string
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func NewClient(cfg Config) *Client {
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if transport == "" {
		transport = "http"
	}
	interval := time.Duration(cfg.RegisterIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	state, outboxErr := loadOutbox(cfg.OutboxPath)
	if outboxErr == nil && cfg.Device != nil && len(state.Queue) > 0 {
		for i := range state.Queue {
			state.Queue[i].Body["deviceId"] = cfg.Device.DeviceID
			state.Queue[i].Body["bootId"] = cfg.Device.BootID
		}
		outboxErr = saveOutbox(cfg.OutboxPath, state)
	}
	return &Client{
		outboxPath:          cfg.OutboxPath,
		outboxErr:           outboxErr,
		seq:                 state.Seq,
		queue:               state.Queue,
		mqttAckWaiters:      make(map[string]chan bool),
		dropped:             state.Dropped,
		gatewayURL:          strings.TrimRight(cfg.GatewayURL, "/"),
		transport:           transport,
		mqttURL:             cfg.MQTTURL,
		mqttClientID:        cfg.MQTTClientID,
		mqttUsername:        cfg.MQTTUsername,
		mqttPassword:        cfg.MQTTPassword,
		mqttCAFile:          cfg.MQTTCAFile,
		mqttClientCertFile:  cfg.MQTTClientCertFile,
		mqttClientKeyFile:   cfg.MQTTClientKeyFile,
		dev:                 cfg.Device,
		endpoint:            cfg.Endpoint,
		token:               cfg.Token,
		registerIntervalSec: interval,
		httpClient: &http.Client{
			Timeout: PostTimeout,
		},
		kickChan: make(chan struct{}, 10),
		stopChan: make(chan struct{}),
	}
}

func (c *Client) Start() {
	if c.outboxErr != nil {
		log.Printf("cannot start gateway client: %v", c.outboxErr)
		return
	}
	if c.transport == "mqtt" {
		go c.startMQTT()
	}
	go c.registerLoop()
	go c.flushLoop()
}

func (c *Client) Stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	cancel := c.mqttCancel
	c.mu.Unlock()
	close(c.stopChan)
	if cancel != nil {
		cancel()
	}
	c.mu.Lock()
	cm := c.mqtt
	c.mu.Unlock()
	if cm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cm.Disconnect(ctx)
	}
}

func (c *Client) Kick() {
	select {
	case c.kickChan <- struct{}{}:
	default:
	}
}

func (c *Client) EnqueueTelemetry(readings []device.Reading) {
	if len(readings) == 0 {
		return
	}
	path := fmt.Sprintf("/devices/%s/telemetry", url.PathEscape(c.dev.DeviceID))
	c.enqueue(path, map[string]interface{}{"readings": readings})
}

func (c *Client) EnqueueEvent(evt device.Event) {
	path := fmt.Sprintf("/devices/%s/events", url.PathEscape(c.dev.DeviceID))
	payload := map[string]interface{}{
		"eventId": generateUUID(),
		"type":    evt.Type,
	}
	if evt.Status != "" {
		payload["status"] = evt.Status
	}
	if evt.Reason != "" {
		payload["reason"] = evt.Reason
	}
	if evt.State != nil {
		payload["state"] = evt.State
	}
	if evt.Cause != nil {
		payload["cause"] = evt.Cause
	}
	c.enqueue(path, payload)
}

func (c *Client) enqueue(path string, payload map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	nextSeq := c.seq + 1
	messageID := generateUUID()
	payload["messageId"] = messageID
	payload["deviceId"] = c.dev.DeviceID
	payload["bootId"] = c.dev.BootID
	payload["seq"] = nextSeq
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	if len(c.queue) >= MaxQueue {
		nextDropped := c.dropped + 1
		if err := c.persistOutboxLocked(c.seq, c.queue, nextDropped); err != nil {
			log.Printf("outbox full and failed to persist drop counter: %v", err)
		} else {
			c.dropped = nextDropped
		}
		log.Printf("outbox full for device %s; rejected message %s", c.dev.DeviceID, messageID)
		return
	}
	nextQueue := append(append([]OutboundMessage(nil), c.queue...), OutboundMessage{MessageID: messageID, Path: path, Body: payload})
	if err := c.persistOutboxLocked(nextSeq, nextQueue, c.dropped); err != nil {
		c.dropped++
		log.Printf("failed to persist outbound message %s: %v", messageID, err)
		return
	}
	c.seq = nextSeq
	c.queue = nextQueue
}

func (c *Client) persistOutboxLocked(seq int64, queue []OutboundMessage, dropped int64) error {
	return saveOutbox(c.outboxPath, outboxState{Version: 1, Seq: seq, Dropped: dropped, Queue: queue})
}

func (c *Client) post(ctx context.Context, path string, body interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	reqURL := c.gatewayURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-Id", c.dev.DeviceID)
	if c.token != "" {
		req.Header.Set("X-Device-Token", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func (c *Client) registerLoop() {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-c.kickChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(0)
		case <-timer.C:
			delay := c.doRegister()
			timer.Reset(delay)
		}
	}
}

func (c *Client) doRegister() time.Duration {
	c.mu.Lock()
	if c.dev.Status == "OFFLINE" {
		c.mu.Unlock()
		return time.Second
	}
	if c.transport == "mqtt" {
		// Every registration/heartbeat must be accepted by the gateway before
		// the client considers itself registered and resumes queue delivery.
		c.registered = false
	}
	c.mu.Unlock()

	body := map[string]interface{}{
		"deviceId": c.dev.DeviceID,
		"bootId":   c.dev.BootID,
		"endpoint": c.endpoint,
		"info":     c.dev.Info(),
		"state":    c.dev.StateView()["state"],
		"status":   c.dev.Status,
		"ts":       time.Now().UTC().Format(time.RFC3339Nano),
	}

	ctx, cancel := context.WithTimeout(context.Background(), PostTimeout)
	defer cancel()

	statusCode := 0
	var err error
	if c.transport == "mqtt" {
		err = c.publishMQTT(ctx, "/devices/register", body)
		if err == nil {
			err = c.waitForMQTTRegistration(ctx)
		}
		if err == nil {
			statusCode = http.StatusAccepted
		}
	} else {
		statusCode, err = c.post(ctx, "/devices/register", body)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err == nil && statusCode >= 200 && statusCode < 300 {
		if !c.registered {
			log.Printf(`{"level":"info","msg":"registered with gateway","transport":"%s","gateway":"%s"}`, c.transport, c.gatewayURL)
		}
		c.registered = true
		c.registerFailures = 0
		return c.registerIntervalSec
	}

	c.registerFailures++
	if c.transport == "mqtt" {
		log.Printf("MQTT registration failed for device %s (attempt %d): %v", c.dev.DeviceID, c.registerFailures, err)
	}
	backoffSec := math.Min(30, math.Pow(2, math.Min(float64(c.registerFailures), 5)))
	return time.Duration(backoffSec) * time.Second
}

func (c *Client) flushLoop() {
	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.doFlush()
		}
	}
}

func (c *Client) doFlush() {
	for {
		c.mu.Lock()
		if len(c.queue) == 0 || !c.registered {
			c.mu.Unlock()
			return
		}
		msg := c.queue[0]
		c.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), PostTimeout)
		statusCode := 0
		var err error
		if c.transport == "mqtt" {
			err = c.publishMQTTAndWaitAck(ctx, msg)
			if err == nil {
				statusCode = http.StatusAccepted
			}
		} else {
			statusCode, err = c.post(ctx, msg.Path, msg.Body)
		}
		cancel()

		c.mu.Lock()
		if err != nil || statusCode == 429 || statusCode >= 500 {
			c.flushFailures++
			c.mu.Unlock()
			return
		}

		if statusCode >= 200 && statusCode < 300 {
			if len(c.queue) > 0 {
				nextQueue := append([]OutboundMessage(nil), c.queue[1:]...)
				if err := c.persistOutboxLocked(c.seq, nextQueue, c.dropped); err != nil {
					c.flushFailures++
					log.Printf("failed to persist outbox acknowledgement: %v", err)
					c.mu.Unlock()
					return
				}
				c.queue = nextQueue
			}
			c.flushFailures = 0
			c.mu.Unlock()
			continue
		}

		if statusCode == 404 {
			log.Printf(`{"level":"warn","msg":"gateway does not know this device, re-registering"}`)
			c.registered = false
			c.mu.Unlock()
			c.Kick()
			return
		}

		// Any other 4xx: drop message
		log.Printf(`{"level":"warn","msg":"gateway refused a message, dropping it","status":%d}`, statusCode)
		if len(c.queue) > 0 {
			nextQueue := append([]OutboundMessage(nil), c.queue[1:]...)
			if err := c.persistOutboxLocked(c.seq, nextQueue, c.dropped); err != nil {
				log.Printf("failed to persist rejected-message removal: %v", err)
				c.mu.Unlock()
				return
			}
			c.queue = nextQueue
		}
		c.mu.Unlock()
	}
}

func (c *Client) Stats() ClientStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ClientStats{
		Registered: c.registered,
		Queued:     len(c.queue),
		Dropped:    c.dropped,
		Seq:        c.seq,
		OutboxPath: c.outboxPath,
	}
}

func (c *Client) IsRegistered() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registered
}
