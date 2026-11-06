package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"

	"fogsense-device-runtime/pkg/gatewaystore"
)

func validateMQTTDeliveryMessage(payload map[string]interface{}, topic, prefix string) (string, string, string) {
	normalizedTopic := strings.Trim(topic, "/")
	normalizedPrefix := strings.Trim(prefix, "/")
	if !strings.HasPrefix(normalizedTopic, normalizedPrefix+"/") {
		return "", "", "topic is outside the configured device namespace"
	}
	relative := strings.TrimPrefix(normalizedTopic, normalizedPrefix+"/")
	parts := strings.Split(relative, "/")
	if len(parts) < 2 || parts[0] == "" {
		return "", "", "topic does not contain a device ID and message type"
	}
	deviceID := parts[0]
	messageID, _ := payload["messageId"].(string)
	if messageID == "" {
		return deviceID, "", "messageId is required"
	}
	bodyDeviceID, _ := payload["deviceId"].(string)
	if bodyDeviceID != deviceID {
		return deviceID, messageID, "deviceId does not match the topic"
	}
	if bootID, ok := payload["bootId"].(string); !ok || bootID == "" {
		return deviceID, messageID, "bootId is required"
	}
	seq, ok := payload["seq"].(float64)
	if !ok || seq < 1 || seq != math.Trunc(seq) {
		return deviceID, messageID, "seq must be a positive integer"
	}
	if ts, ok := payload["ts"].(string); !ok || ts == "" {
		return deviceID, messageID, "ts is required"
	}
	messageType := strings.Join(parts[1:], "/")
	if messageType != "telemetry" && messageType != "events" && messageType != "commands/ack" {
		return deviceID, messageID, "unsupported device publication topic"
	}
	return deviceID, messageID, ""
}

func maintainSubscription(ctx context.Context, cm *autopaho.ConnectionManager, topic string, ready *atomic.Bool) {
	backoff := time.Second
	for {
		subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := cm.Subscribe(subCtx, &paho.Subscribe{
			Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: 1}},
		})
		cancel()
		if err == nil {
			ready.Store(true)
			log.Printf("subscribed to %s", topic)
			return
		}
		log.Printf("subscribe failed: %v; retrying in %s", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func main() {
	brokerURL := flag.String("broker-url", "mqtt://127.0.0.1:1883", "MQTT 5 broker URL (mqtt:// or mqtts://)")
	clientID := flag.String("client-id", "fogsense-mock-gateway", "MQTT client identifier")
	username := flag.String("username", "", "optional MQTT username")
	password := flag.String("password", "", "optional MQTT password")
	caFile := flag.String("ca-file", "", "optional PEM CA bundle used to verify the broker")
	clientCertFile := flag.String("client-cert-file", os.Getenv("MQTT_GATEWAY_CLIENT_CERT_FILE"), "optional MQTT client certificate for mutual TLS")
	clientKeyFile := flag.String("client-key-file", os.Getenv("MQTT_GATEWAY_CLIENT_KEY_FILE"), "optional MQTT client private key for mutual TLS")
	prefix := flag.String("topic-prefix", "fogsense/v1/devices", "device topic prefix")
	storePath := filepath.Join("data", "mqtt-gateway.db")
	storeFile := flag.String("store-file", storePath, "SQLite database for accepted MQTT messages")
	flag.StringVar(storeFile, "dedupe-file", storePath, "deprecated alias for --store-file")
	healthPort := flag.Int("health-port", 8081, "HTTP health endpoint port")
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
	if *clientCertFile == "" != (*clientKeyFile == "") {
		log.Fatal("MQTT_GATEWAY_CLIENT_CERT_FILE and MQTT_GATEWAY_CLIENT_KEY_FILE must be set together")
	}
	if *caFile != "" || *clientCertFile != "" {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if *caFile != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil { log.Fatalf("read CA file: %v", err) }
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil { roots = x509.NewCertPool() }
		if !roots.AppendCertsFromPEM(pem) { log.Fatal("CA file contains no certificates") }
		tlsConfig.RootCAs = roots
	}
	if *clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(*clientCertFile, *clientKeyFile)
		if err != nil { log.Fatalf("load MQTT client certificate: %v", err) }
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	var ready atomic.Bool
	var dedupeHealthy atomic.Bool
	var subscribing atomic.Bool
	messageStore, err := gatewaystore.Open(*storeFile)
	if err != nil {
		log.Fatalf("open MQTT gateway message store: %v", err)
	}
	defer messageStore.Close()
	dedupeHealthy.Store(true)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !ready.Load() || !dedupeHealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"DOWN","reason":"MQTT subscription or deduplication storage is not ready"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"UP","mqttSubscribed":true}`))
	})
	healthServer := &http.Server{Addr: fmt.Sprintf(":%d", *healthPort), Handler: healthMux}
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("health server failed: %v", err)
		}
	}()

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
			OnClientError: func(err error) {
				ready.Store(false)
				log.Printf("MQTT client error: %v", err)
			},
			OnServerDisconnect: func(disconnect *paho.Disconnect) {
				ready.Store(false)
				log.Printf("MQTT broker disconnected: reason=%d", disconnect.ReasonCode)
			},
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
					} else if !strings.HasSuffix(received.Packet.Topic, "/delivery/ack") &&
						!strings.HasSuffix(received.Packet.Topic, "/register/ack") &&
						!strings.HasSuffix(received.Packet.Topic, "/commands") {
						if messageID, ok := payload["messageId"].(string); ok && messageID != "" {
							deviceID, messageID, validationError := validateMQTTDeliveryMessage(payload, received.Packet.Topic, *prefix)
							if deviceID == "" {
								log.Printf("cannot accept message on malformed topic %s", received.Packet.Topic)
								return true, nil
							}
							if validationError != "" {
								ack, _ := json.Marshal(map[string]interface{}{
									"deviceId": deviceID,
									"messageId": messageID,
									"accepted": false,
									"reason": validationError,
								})
								ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
								defer cancel()
								ackTopic := strings.TrimRight(*prefix, "/") + "/" + deviceID + "/delivery/ack"
								if _, err := received.Client.Publish(ackCtx, &paho.Publish{QoS: 1, Topic: ackTopic, Payload: ack}); err != nil {
									log.Printf("failed to reject malformed delivery on %s: %v", ackTopic, err)
								}
								return true, nil
							}
							relativeTopic := strings.TrimPrefix(strings.Trim(received.Packet.Topic, "/"), strings.TrimRight(*prefix, "/")+"/")
							topicParts := strings.Split(relativeTopic, "/")
							kind := strings.Join(topicParts[1:], "/")
							duplicate, err := messageStore.Accept(ctx, deviceID, messageID, kind, payload)
							if err != nil {
								dedupeHealthy.Store(false)
								log.Printf("cannot persist %s message %s from %s: %v", kind, messageID, deviceID, err)
								return true, nil
							}
							dedupeHealthy.Store(true)
							if duplicate {
								log.Printf("duplicate MQTT message %s from %s; acknowledging without reprocessing", messageID, deviceID)
							}
							ack, _ := json.Marshal(map[string]interface{}{
								"deviceId": deviceID,
								"messageId": messageID,
								"accepted": true,
							})
							ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
							defer cancel()
							ackTopic := strings.TrimRight(*prefix, "/") + "/" + deviceID + "/delivery/ack"
							if _, err := received.Client.Publish(ackCtx, &paho.Publish{
								QoS: 1, Topic: ackTopic, Payload: ack,
							}); err != nil {
								log.Printf("failed to acknowledge delivery on %s: %v", ackTopic, err)
							}
						}
					}
					return true, nil
				},
			},
		},
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			if !subscribing.CompareAndSwap(false, true) {
				return
			}
			ready.Store(false)
			topic := strings.TrimRight(*prefix, "/") + "/#"
			go func() {
				defer subscribing.Store(false)
				maintainSubscription(ctx, cm, topic, &ready)
			}()
		},
		OnConnectError: func(err error) {
			ready.Store(false)
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

	ready.Store(false)
	disconnectCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = cm.Disconnect(disconnectCtx)
	_ = healthServer.Shutdown(disconnectCtx)
}
