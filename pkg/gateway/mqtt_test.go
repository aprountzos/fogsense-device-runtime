package gateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fogsense-device-runtime/pkg/device"
	"fogsense-device-runtime/pkg/spec"

	"github.com/eclipse/paho.golang/paho"
)

func TestMQTTTopic(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"registration", "/devices/register", "fogsense/v1/devices/light-1/register"},
		{"telemetry", "/devices/light-1/telemetry", "fogsense/v1/devices/light-1/telemetry"},
		{"events", "/devices/light-1/events", "fogsense/v1/devices/light-1/events"},
		{"command acknowledgement", "/devices/commands/ack", "fogsense/v1/devices/light-1/commands/ack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mqttTopic("light-1", tt.path)
			if err != nil {
				t.Fatalf("mqttTopic() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("mqttTopic() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMQTTTopicRejectsUnknownPath(t *testing.T) {
	if _, err := mqttTopic("light-1", "/unknown"); err == nil {
		t.Fatal("expected unsupported path to return an error")
	}
}

func TestNewClientDefaultsToHTTPTransport(t *testing.T) {
	client := NewClient(Config{})
	if client.transport != "http" {
		t.Fatalf("transport = %q, want http", client.transport)
	}
}

func TestNewClientSelectsMQTTTransport(t *testing.T) {
	client := NewClient(Config{Transport: " MQTT "})
	if client.transport != "mqtt" {
		t.Fatalf("transport = %q, want mqtt", client.transport)
	}
}

func TestMQTTRegistrationAckRequiresMatchingDeviceBoot(t *testing.T) {
	dev := &device.Device{DeviceID: "light-1", BootID: "boot-current"}
	client := NewClient(Config{Device: dev})

	client.handleMQTTRegistrationAck([]byte(`{"deviceId":"light-1","bootId":"boot-old","accepted":true}`))
	if client.IsRegistered() {
		t.Fatal("registration acknowledgement for another boot must be ignored")
	}

	client.handleMQTTRegistrationAck([]byte(`{"deviceId":"light-1","bootId":"boot-current","accepted":true}`))
	if !client.IsRegistered() {
		t.Fatal("matching accepted registration acknowledgement should register the device")
	}
}

func TestMQTTCommandAcknowledgementIsQueued(t *testing.T) {
	dev := &device.Device{DeviceID: "light-1", BootID: "boot-current", Spec: &spec.DeviceSpec{}}
	client := NewClient(Config{Device: dev, Transport: "mqtt"})
	received := paho.PublishReceived{
		Packet: &paho.Publish{Payload: []byte(`{"commandId":"cmd-1","name":"TURN_ON"}`)},
	}

	if !client.handleMQTTCommand(received) {
		t.Fatal("command should be handled when its acknowledgement is durably queued")
	}

	acks := dev.PendingCommandAcks()
	if len(acks) != 1 {
		t.Fatalf("pending command acknowledgements = %d, want 1", len(acks))
	}
	ack := acks[0]
	if ack.CommandID != "cmd-1" {
		t.Fatalf("commandId = %q, want cmd-1", ack.CommandID)
	}
	if ack.MessageID == "" {
		t.Fatal("expected a stable message ID for the pending acknowledgement")
	}
	if ack.Status < 100 || ack.Status > 599 {
		t.Fatalf("invalid acknowledgement status: %d", ack.Status)
	}
	if ack.Response == nil {
		t.Fatal("expected command response to be persisted")
	}
}

func TestMQTTHeartbeatRequiresFreshRegistrationAck(t *testing.T) {
	dev := &device.Device{DeviceID: "light-1", BootID: "boot-current", Spec: &spec.DeviceSpec{}}
	client := NewClient(Config{Device: dev, Transport: "mqtt"})
	client.registered = true

	// No active MQTT connection: registration will fail, but must first clear
	// the previous registration state so queued messages cannot flush.
	client.doRegister()

	if client.IsRegistered() {
		t.Fatal("client must wait for a fresh application acknowledgement")
	}
}

func TestMQTTDeliveryAcknowledgementMatchesDeviceAndMessage(t *testing.T) {
	dev := &device.Device{DeviceID: "light-1", BootID: "boot-current"}
	client := NewClient(Config{Device: dev, Transport: "mqtt"})
	waiter := make(chan bool, 1)
	client.mqttAckWaiters["message-1"] = waiter

	client.handleMQTTDeliveryAck([]byte(`{"deviceId":"other-device","messageId":"message-1","accepted":true}`))
	select {
	case <-waiter:
		t.Fatal("acknowledgement for another device must be ignored")
	default:
	}

	client.handleMQTTDeliveryAck([]byte(`{"deviceId":"light-1","messageId":"message-1","accepted":true}`))
	select {
	case accepted := <-waiter:
		if !accepted {
			t.Fatal("expected accepted acknowledgement")
		}
	default:
		t.Fatal("expected matching acknowledgement to notify waiter")
	}
}

func TestMQTTDeliveryAcknowledgementCanRejectMessage(t *testing.T) {
	dev := &device.Device{DeviceID: "light-1", BootID: "boot-current"}
	client := NewClient(Config{Device: dev, Transport: "mqtt"})
	waiter := make(chan bool, 1)
	client.mqttAckWaiters["message-2"] = waiter

	client.handleMQTTDeliveryAck([]byte(`{"deviceId":"light-1","messageId":"message-2","accepted":false}`))
	if accepted := <-waiter; accepted {
		t.Fatal("expected gateway rejection")
	}
}

func TestBuildMQTTTLSConfigLoadsCustomCA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{CommonName: "FogSense Test CA"},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour),
		IsCA: true,
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := buildMQTTTLSConfig(caPath, "", "")
	if err != nil {
		t.Fatalf("build TLS config: %v", err)
	}
	if cfg == nil || cfg.RootCAs == nil {
		t.Fatal("expected custom root CA pool")
	}
	if cfg.MinVersion != 0x0303 {
		t.Fatalf("minimum TLS version = %x, want TLS 1.2 (0x0303)", cfg.MinVersion)
	}
}

func TestBuildMQTTTLSConfigRequiresClientCertificatePair(t *testing.T) {
	if _, err := buildMQTTTLSConfig("", "client.crt", ""); err == nil {
		t.Fatal("expected missing client key to fail")
	}
	if _, err := buildMQTTTLSConfig("", "", "client.key"); err == nil {
		t.Fatal("expected missing client certificate to fail")
	}
}
