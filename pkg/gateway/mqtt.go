package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

const mqttTopicPrefix = "fogsense/v1/devices"

var errMQTTRejected = errors.New("gateway application rejected MQTT message")

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
	u, err := url.Parse(brokerURL)
	if err != nil || (u.Scheme != "mqtt" && u.Scheme != "tls") || u.Host == "" {
		log.Printf("invalid MQTT broker URL %q: expected mqtt:// or mqtts://", c.mqttURL)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.mqttCancel = cancel
	c.mu.Unlock()

	tlsConfig, tlsErr := buildMQTTTLSConfig(c.mqttCAFile, c.mqttClientCertFile, c.mqttClientKeyFile)
	if tlsErr != nil {
		log.Printf("MQTT TLS configuration failed: %v", tlsErr)
		return
	}
	cfg := autopaho.ClientConfig{
		TlsCfg:                        tlsConfig,
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     30,
		CleanStartOnInitialConnection: false,
		SessionExpiryInterval:         86400,
		ConnectUsername:               c.mqttUsername,
		ConnectPassword:               []byte(c.mqttPassword),
		ClientConfig: paho.ClientConfig{
			ClientID: c.mqttClientID,
			EnableManualAcknowledgment: true,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(received paho.PublishReceived) (bool, error) {
					if strings.HasSuffix(received.Packet.Topic, "/register/ack") {
						c.handleMQTTRegistrationAck(received.Packet.Payload)
					} else if strings.HasSuffix(received.Packet.Topic, "/commands") {
						// Do not acknowledge the broker delivery until the command
						// result is durably queued for delivery to the gateway.
						if !c.handleMQTTCommand(received.Packet.Payload) {
							return false, nil
						}
					} else if strings.HasSuffix(received.Packet.Topic, "/delivery/ack") {
						c.handleMQTTDeliveryAck(received.Packet.Payload)
					}
					if err := received.Client.Ack(received.Packet); err != nil {
						return false, fmt.Errorf("acknowledge MQTT delivery: %w", err)
					}
					return true, nil
				},
			},
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			c.mu.Lock()
			c.mqtt = cm
			c.mqttReady = false
			c.registered = false
			c.mu.Unlock()
			commandsTopic := fmt.Sprintf("%s/%s/commands", mqttTopicPrefix, c.dev.DeviceID)
			registrationAckTopic := fmt.Sprintf("%s/%s/register/ack", mqttTopicPrefix, c.dev.DeviceID)
			deliveryAckTopic := fmt.Sprintf("%s/%s/delivery/ack", mqttTopicPrefix, c.dev.DeviceID)
			go c.subscribeMQTT(cm, commandsTopic, registrationAckTopic, deliveryAckTopic)
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
	stopped := c.stopped
	if !stopped {
		c.mqtt = cm
	}
	c.mu.Unlock()
	if stopped {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer disconnectCancel()
		_ = cm.Disconnect(disconnectCtx)
	}
}

func (c *Client) subscribeMQTT(cm *autopaho.ConnectionManager, commandsTopic, registrationAckTopic, deliveryAckTopic string) {
	delay := time.Second
	for {
		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if stopped {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := cm.Subscribe(ctx, &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{
				{Topic: commandsTopic, QoS: 1},
				{Topic: registrationAckTopic, QoS: 1},
				{Topic: deliveryAckTopic, QoS: 1},
			},
		})
		cancel()
		if err == nil {
			c.mu.Lock()
			if !c.stopped && c.mqtt == cm {
				c.mqttReady = true
				c.registered = false
			}
			c.mu.Unlock()
			c.Kick()
			log.Printf("MQTT connected; subscribed to %s, %s and %s", commandsTopic, registrationAckTopic, deliveryAckTopic)
			return
		}

		log.Printf("MQTT subscription failed; retrying: %v", err)
		select {
		case <-c.stopChan:
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
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
	ready := c.mqttReady
	c.mu.Unlock()
	if cm == nil || !ready {
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

func (c *Client) waitForMQTTRegistration(ctx context.Context) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		c.mu.Lock()
		registered := c.registered
		c.mu.Unlock()
		if registered {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for MQTT registration acknowledgement: %w", ctx.Err())
		case <-ticker.C:
		}
	}
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

func (c *Client) handleMQTTCommand(payload []byte) bool {
	status, response := c.dev.ExecuteCommand(payload)
	if status == 503 && response["reason"] == "PERSISTENCE_FAILED" {
		// The command was not committed. Leave the broker delivery unacknowledged.
		return false
	}
	var command map[string]interface{}
	_ = json.Unmarshal(payload, &command)
	commandID, _ := command["commandId"].(string)

	// Persist the command result in the device database before acknowledging
	// the inbound broker delivery. The main loop transfers it to the outbox.
	if _, err := c.dev.QueueCommandAck(commandID, status, response); err != nil {
		log.Printf("cannot persist MQTT command acknowledgement for %q: %v", commandID, err)
		return false
	}
	return true
}

func (c *Client) publishMQTTAndWaitAck(ctx context.Context, msg OutboundMessage) error {
	ack := make(chan bool, 1)
	c.mu.Lock()
	if c.mqttAckWaiters == nil {
		c.mqttAckWaiters = make(map[string]chan bool)
	}
	c.mqttAckWaiters[msg.MessageID] = ack
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.mqttAckWaiters, msg.MessageID)
		c.mu.Unlock()
	}()

	if err := c.publishMQTT(ctx, msg.Path, msg.Body); err != nil {
		return err
	}
	select {
	case accepted := <-ack:
		if !accepted {
			return fmt.Errorf("%w: %s", errMQTTRejected, msg.MessageID)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for gateway application acknowledgement for %s: %w", msg.MessageID, ctx.Err())
	}
}

func (c *Client) handleMQTTDeliveryAck(payload []byte) {
	var ack struct {
		DeviceID  string `json:"deviceId"`
		MessageID string `json:"messageId"`
		Accepted  bool   `json:"accepted"`
	}
	if err := json.Unmarshal(payload, &ack); err != nil {
		log.Printf("invalid MQTT delivery acknowledgement: %v", err)
		return
	}
	if ack.DeviceID != c.dev.DeviceID || ack.MessageID == "" {
		log.Printf("ignoring MQTT delivery acknowledgement with mismatched device or empty messageId")
		return
	}
	c.mu.Lock()
	waiter := c.mqttAckWaiters[ack.MessageID]
	c.mu.Unlock()
	if waiter == nil {
		return
	}
	select {
	case waiter <- ack.Accepted:
	default:
	}
}

func buildMQTTTLSConfig(caFile, certFile, keyFile string) (*tls.Config, error) {
	if caFile == "" && certFile == "" && keyFile == "" {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read MQTT CA file: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if ok := roots.AppendCertsFromPEM(pem); !ok {
			return nil, fmt.Errorf("MQTT CA file %q contains no certificates", caFile)
		}
		cfg.RootCAs = roots
	}
	if (certFile == "") != (keyFile == "") {
		return nil, fmt.Errorf("MQTT_CLIENT_CERT_FILE and MQTT_CLIENT_KEY_FILE must be set together")
	}
	if certFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load MQTT client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func ValidateMQTTTLSConfig(caFile, certFile, keyFile string) error {
	_, err := buildMQTTTLSConfig(caFile, certFile, keyFile)
	return err
}
