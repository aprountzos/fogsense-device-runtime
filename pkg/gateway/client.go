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
	Path string
	Body map[string]interface{}
}

type ClientStats struct {
	Registered bool  `json:"registered"`
	Queued     int   `json:"queued"`
	Dropped    int64 `json:"dropped"`
	Seq        int64 `json:"seq"`
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
	mqtt                *autopaho.ConnectionManager
	mqttReady           bool
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

	return &Client{
		gatewayURL:          strings.TrimRight(cfg.GatewayURL, "/"),
		transport:           transport,
		mqttURL:             cfg.MQTTURL,
		mqttClientID:        cfg.MQTTClientID,
		mqttUsername:        cfg.MQTTUsername,
		mqttPassword:        cfg.MQTTPassword,
		dev:                 cfg.Device,
		endpoint:            cfg.Endpoint,
		token:               cfg.Token,
		registerIntervalSec: interval,
		queue:               make([]OutboundMessage, 0, MaxQueue),
		httpClient: &http.Client{
			Timeout: PostTimeout,
		},
		kickChan: make(chan struct{}, 10),
		stopChan: make(chan struct{}),
	}
}

func (c *Client) Start() {
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

	c.seq++
	payload["deviceId"] = c.dev.DeviceID
	payload["bootId"] = c.dev.BootID
	payload["seq"] = c.seq
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	c.queue = append(c.queue, OutboundMessage{Path: path, Body: payload})
	for len(c.queue) > MaxQueue {
		c.queue = c.queue[1:]
		c.dropped++
	}
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
			err = c.publishMQTT(ctx, msg.Path, msg.Body)
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
				c.queue = c.queue[1:]
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
			c.queue = c.queue[1:]
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
	}
}

func (c *Client) IsRegistered() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registered
}
