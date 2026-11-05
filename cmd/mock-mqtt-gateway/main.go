package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

func main() {
	brokerURL := flag.String("broker-url", "mqtt://127.0.0.1:1883", "MQTT 5 broker URL (mqtt:// or mqtts://)")
	clientID := flag.String("client-id", "fogsense-mock-gateway", "MQTT client identifier")
	username := flag.String("username", "", "optional MQTT username")
	password := flag.String("password", "", "optional MQTT password")
	caFile := flag.String("ca-file", "", "optional PEM CA bundle used to verify the broker")
	prefix := flag.String("topic-prefix", "fogsense/v1/devices", "device topic prefix")
	flag.Parse()

	if (*username == "") != (*password == "") {
		log.Fatal("username and password must either both be set or both be empty")
	}
	rawURL := *brokerURL
	if strings.HasPrefix(rawURL, "mqtts://") {
		rawURL = "tls://" + strings.TrimPrefix(rawURL, "mqtts://")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "mqtt" && u.Scheme != "tls") {
		log.Fatal("broker URL must use mqtt:// or mqtts://")
	}

	var tlsConfig *tls.Config
	if *caFile != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil { log.Fatalf("read CA file: %v", err) }
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil { roots = x509.NewCertPool() }
		if !roots.AppendCertsFromPEM(pem) { log.Fatal("CA file contains no certificates") }
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := autopaho.ClientConfig{
		TlsCfg:                        tlsConfig,
		ServerUrls:                    []*url.URL{u},
		KeepAlive:                     30,
		CleanStartOnInitialConnection: true,
		ConnectUsername:               *username,
		ConnectPassword:               []byte(*password),
		ClientConfig: paho.ClientConfig{
			ClientID: *clientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(received paho.PublishReceived) (bool, error) {
					var payload map[string]interface{}
					if err := json.Unmarshal(received.Packet.Payload, &payload); err != nil {
						log.Printf("invalid JSON on %s: %v", received.Packet.Topic, err)
						return true, nil
					}
					if payload["deviceId"] == nil {
						log.Printf("message without deviceId on %s", received.Packet.Topic)
					}
					encoded, _ := json.Marshal(payload)
					log.Printf("MQTT %s %s", received.Packet.Topic, encoded)
					if strings.HasSuffix(received.Packet.Topic, "/register") {
						deviceID, _ := payload["deviceId"].(string)
						bootID, _ := payload["bootId"].(string)
						ack, _ := json.Marshal(map[string]interface{}{
							"deviceId": deviceID,
							"bootId": bootID,
							"accepted": deviceID != "" && bootID != "",
						})
						ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
						defer cancel()
						ackTopic := strings.TrimSuffix(received.Packet.Topic, "/register") + "/register/ack"
						if _, err := received.Client.Publish(ackCtx, &paho.Publish{
							QoS: 1, Topic: ackTopic, Payload: ack,
						}); err != nil {
							log.Printf("failed to acknowledge registration on %s: %v", ackTopic, err)
						}
					} else if messageID, ok := payload["messageId"].(string); ok && messageID != "" {
						deviceID, _ := payload["deviceId"].(string)
						ack, _ := json.Marshal(map[string]interface{}{
							"deviceId": deviceID,
							"messageId": messageID,
							"accepted": deviceID != "",
						})
						ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
						defer cancel()
						parts := strings.Split(strings.Trim(received.Packet.Topic, "/"), "/")
						if len(parts) < 4 {
							log.Printf("cannot acknowledge message on malformed topic %q", received.Packet.Topic)
							return true, nil
						}
						ackTopic := strings.Join(parts[:4], "/") + "/delivery/ack"
						if _, err := received.Client.Publish(ackCtx, &paho.Publish{
							QoS: 1, Topic: ackTopic, Payload: ack,
						}); err != nil {
							log.Printf("failed to acknowledge delivery on %s: %v", ackTopic, err)
						}
					}
					return true, nil
				},
			},
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			topic := strings.TrimRight(*prefix, "/") + "/#"
			subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_, err := cm.Subscribe(subCtx, &paho.Subscribe{
				Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}},
			})
			if err != nil {
				log.Printf("subscribe failed: %v", err)
				return
			}
			log.Printf("subscribed to %s", topic)
		},
		OnConnectError: func(err error) {
			log.Printf("MQTT connection failed: %v", err)
		},
	}

	cm, err := autopaho.NewConnection(ctx, cfg)
	if err != nil {
		log.Fatal(fmt.Errorf("create MQTT client: %w", err))
	}
	if err := cm.AwaitConnection(ctx); err != nil {
		log.Printf("MQTT connection ended: %v", err)
		return
	}

	// AwaitConnection only confirms the initial connection. Keep the gateway
	// alive so it can receive registrations, telemetry, events and commands,
	// and let autopaho handle reconnects until the process is stopped.
	<-ctx.Done()

	disconnectCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = cm.Disconnect(disconnectCtx)
}
