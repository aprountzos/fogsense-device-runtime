package gateway

import (
	"testing"

	"fogsense-device-runtime/pkg/device"
	"fogsense-device-runtime/pkg/spec"
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

	client.handleMQTTCommand([]byte(`{"commandId":"cmd-1","name":"TURN_ON"}`))

	if len(client.queue) != 1 {
		t.Fatalf("queued messages = %d, want 1", len(client.queue))
	}
	msg := client.queue[0]
	if msg.Path != "/devices/commands/ack" {
		t.Fatalf("queued path = %q, want command acknowledgement path", msg.Path)
	}
	if got := msg.Body["deviceId"]; got != "light-1" {
		t.Fatalf("deviceId = %v, want light-1", got)
	}
	if got := msg.Body["bootId"]; got != "boot-current" {
		t.Fatalf("bootId = %v, want boot-current", got)
	}
	if got := msg.Body["commandId"]; got != "cmd-1" {
		t.Fatalf("commandId = %v, want cmd-1", got)
	}
	if got := msg.Body["seq"]; got != int64(1) {
		t.Fatalf("seq = %v, want 1", got)
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
