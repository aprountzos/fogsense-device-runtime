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
	brokerURL := c.mqttURL
	if strings.HasPrefix(brokerURL, "mqtts://") {
		brokerURL = "tls://" + strings.TrimPrefix(brokerURL, "mqtts://")
	}
	u, err := url.Parse(brokerURL)
	if err != nil {
		log.Printf("invalid MQTT broker URL: %v", err)
		return
	}

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
					c.handleMQTTCommand(received.Packet.Payload)
					return true, nil
				},
			},
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			topic := fmt.Sprintf("%s/%s/commands", mqttTopicPrefix, c.dev.DeviceID)
			subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer subCancel()
			if _, err := cm.Subscribe(subCtx, &paho.Subscribe{
				Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}},
			}); err != nil {
				log.Printf("MQTT command subscription failed: %v", err)
				return
			}
			c.mu.Lock()
			c.mqtt = cm
			c.mu.Unlock()
			log.Printf("MQTT connected; subscribed to %s", topic)
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

func (c *Client) publishMQTT(ctx context.Context, path string, body interface{}) error {
	c.mu.Lock()
	cm := c.mqtt
	c.mu.Unlock()
	if cm == nil {
		return fmt.Errorf("MQTT connection is not ready")
	}

	var messageType string
	switch {
	case path == "/devices/register":
		messageType = "register"
	case strings.HasSuffix(path, "/telemetry"):
		messageType = "telemetry"
	case strings.HasSuffix(path, "/events"):
		messageType = "events"
	default:
		return fmt.Errorf("unsupported MQTT gateway path %q", path)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	topic := fmt.Sprintf("%s/%s/%s", mqttTopicPrefix, c.dev.DeviceID, messageType)
	_, err = cm.Publish(ctx, &paho.Publish{QoS: 1, Topic: topic, Payload: payload})
	return err
}

func (c *Client) handleMQTTCommand(payload []byte) {
	status, response := c.dev.ExecuteCommand(payload)
	result := map[string]interface{}{
		"httpStatus": status,
		"response":   response,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.publishMQTT(ctx, "/devices/commands-result", map[string]interface{}{
		"deviceId": c.dev.DeviceID,
		"result":   result,
	}); err != nil {
		log.Printf("failed to publish MQTT command result: %v", err)
	}
}
