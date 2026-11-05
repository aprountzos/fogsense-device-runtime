package gateway

import "testing"

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
