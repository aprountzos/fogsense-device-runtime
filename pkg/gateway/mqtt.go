package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

const mqttTopicPrefix = "fogsense/v1/devices"

func (c *Client) startMQTT() {
	if c.mqttClientID == "" {
		c.mqttClientID = "fogsense-" + c.dev.DeviceID
	}
	brokerURL := c.mqttURL
	if strings.HasPrefix(brokerURL, "mqtts://") {
		brokerURL = "tls://" + strings.TrimPrefix(brokerURL, "mqtts://")
	} else if strings.HasPrefix(brokerURL, "tcp://") {
		brokerURL = "mqtt://" + strings.TrimPrefix(brokerURL, "tcp://")
	}
	if u, err := url.Parse(brokerURL); err != nil || (u.Scheme != "mqtt" && u.Scheme != "tls" && u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		log.Printf("invalid MQTT broker URL %q: expected mqtt:// or mqtts:// (optionally ws:// or wss://)", c.mqttURL)
		return
	}
	u, _ := url.Parse(brokerURL)

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.mqttCancel = cancel
	c.mu.Unlock()

	cfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     30,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         60,
		ConnectUsername:               c.mqttUsername,
		ConnectPassword:               []byte(c.mqttPassword),
		ClientConfig: paho.ClientConfig{
			ClientID: c.mqttClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(received paho.PublishReceived) (bool, error) {
					if strings.HasSuffix(received.Packet.Topic, "/register/ack") {
						c.handleMQTTRegistrationAck(received.Packet.Payload)
					} else if strings.HasSuffix(received.Packet.Topic, "/commands") {
						c.handleMQTTCommand(received.Packet.Payload)
					}
					return true, nil
				},
			},
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			c.mu.Lock()
			c.mqtt = cm
			c.mu.Unlock()
			commandsTopic := fmt.Sprintf("%s/%s/commands", mqttTopicPrefix, c.dev.DeviceID)
			registrationAckTopic := fmt.Sprintf("%s/%s/register/ack", mqttTopicPrefix, c.dev.DeviceID)
			subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer subCancel()
			if _, err := cm.Subscribe(subCtx, &paho.Subscribe{
				Subscriptions: []paho.SubscribeOptions{
					{Topic: commandsTopic, QoS: 1},
					{Topic: registrationAckTopic, QoS: 1},
				},
			}); err != nil {
				log.Printf("MQTT command subscription failed: %v", err)
				return
			}
			log.Printf("MQTT connected; subscribed to %s and %s", commandsTopic, registrationAckTopic)
		},
		OnConnectionDown: func() bool {
			c.mu.Lock()
			stopped := c.stopped
			c.registered = false
			c.mu.Unlock()
			if !stopped {
				c.Kick()
			}
			return !stopped
		},
		OnConnectError: func(err error) {
			log.Printf("MQTT connection attempt failed: %v", err)
		},
	}

	cm, err := autopaho.NewConnection(ctx, cfg)
	if err != nil {
		log.Printf("MQTT client initialization failed: %v", err)
		return
	}
	c.mu.Lock()
	c.mqtt = cm
	c.mu.Unlock()
}

func mqttTopic(deviceID, path string) (string, error) {
	var messageType string
	switch {
	case path == "/devices/register":
		messageType = "register"
	case strings.HasSuffix(path, "/telemetry"):
		messageType = "telemetry"
	case strings.HasSuffix(path, "/events"):
		messageType = "events"
	case strings.HasSuffix(path, "/commands/ack"):
		messageType = "commands/ack"
	default:
		return "", fmt.Errorf("unsupported MQTT gateway path %q", path)
	}
	return fmt.Sprintf("%s/%s/%s", mqttTopicPrefix, deviceID, messageType), nil
}

func (c *Client) publishMQTT(ctx context.Context, path string, body interface{}) error {
	c.mu.Lock()
	cm := c.mqtt
	c.mu.Unlock()
	if cm == nil {
		return fmt.Errorf("MQTT connection is not ready")
	}

	topic, err := mqttTopic(c.dev.DeviceID, path)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = cm.Publish(ctx, &paho.Publish{QoS: 1, Topic: topic, Payload: payload})
	return err
}

func (c *Client) handleMQTTRegistrationAck(payload []byte) {
	var ack struct {
		DeviceID string `json:"deviceId"`
		BootID   string `json:"bootId"`
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal(payload, &ack); err != nil {
		log.Printf("invalid MQTT registration acknowledgement: %v", err)
		return
	}
	if !ack.Accepted || ack.DeviceID != c.dev.DeviceID || ack.BootID != c.dev.BootID {
		log.Printf("MQTT registration acknowledgement rejected or did not match this device boot")
		return
	}
	c.mu.Lock()
	wasRegistered := c.registered
	c.registered = true
	c.registerFailures = 0
	c.mu.Unlock()
	if !wasRegistered {
		log.Printf("registered with MQTT gateway")
	}
}

func (c *Client) handleMQTTCommand(payload []byte) {
	status, response := c.dev.ExecuteCommand(payload)
	var command map[string]interface{}
	_ = json.Unmarshal(payload, &command)
	result := map[string]interface{}{
		"deviceId":  c.dev.DeviceID,
		"commandId": command["commandId"],
		"status":    status,
		"response":  response,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.publishMQTT(ctx, "/devices/commands/ack", result); err != nil {
		log.Printf("failed to publish MQTT command result: %v", err)
	}
}
