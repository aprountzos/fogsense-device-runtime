package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fogsense-device-runtime/pkg/device"
	"fogsense-device-runtime/pkg/gateway"
	"fogsense-device-runtime/pkg/server"
	"fogsense-device-runtime/pkg/spec"
)

var deviceIDRegex = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func logJSON(level, msg string, extra map[string]interface{}) {
	entry := map[string]interface{}{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level,
		"msg":   msg,
	}
	for k, v := range extra {
		entry[k] = v
	}
	data, _ := json.Marshal(entry)
	fmt.Println(string(data))
}

func fatal(msg string, extra map[string]interface{}) {
	logJSON("error", msg, extra)
	os.Exit(2)
}

func loadSpecRaw() []byte {
	if b64 := os.Getenv("DEVICE_SPEC_B64"); b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			fatal("DEVICE_SPEC_B64 is not valid base64", map[string]interface{}{"error": err.Error()})
		}
		return data
	}

	if s := os.Getenv("DEVICE_SPEC"); s != "" {
		return []byte(s)
	}

	filePath := os.Getenv("DEVICE_SPEC_FILE")
	if filePath == "" {
		filePath = "/config/spec.yaml"
	}

	data, err := os.ReadFile(filePath)
	if err != nil && filePath == "/config/spec.yaml" {
		if d2, err2 := os.ReadFile("/config/spec.yml"); err2 == nil {
			return d2
		}
		if d3, err3 := os.ReadFile("/config/spec.json"); err3 == nil {
			return d3
		}
	}
	if err != nil {
		fatal("no device spec found: set DEVICE_SPEC_B64, DEVICE_SPEC or DEVICE_SPEC_FILE", map[string]interface{}{"file": filePath})
	}
	return data
}

func main() {
	deviceID := os.Getenv("DEVICE_ID")
	if !deviceIDRegex.MatchString(deviceID) {
		fatal("DEVICE_ID is required and must match [A-Za-z0-9_.-]{1,64}", nil)
	}

	gatewayURL := strings.TrimRight(os.Getenv("GATEWAY_URL"), "/")
	if !strings.HasPrefix(gatewayURL, "http://") && !strings.HasPrefix(gatewayURL, "https://") {
		fatal("GATEWAY_URL is required and must start with http:// or https://", nil)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	timeScale := 1.0
	if tsStr := os.Getenv("TIME_SCALE"); tsStr != "" {
		ts, err := strconv.ParseFloat(tsStr, 64)
		if err != nil || ts <= 0 || ts > 1000 {
			fatal("TIME_SCALE must be a number in (0, 1000]", nil)
		}
		timeScale = ts
	}

	var seedPtr *int64
	if sStr := os.Getenv("SEED"); sStr != "" {
		s, err := strconv.ParseInt(sStr, 10, 64)
		if err == nil {
			seedPtr = &s
		}
	}

	registerIntervalSec := 30
	if regStr := os.Getenv("REGISTER_INTERVAL_SEC"); regStr != "" {
		if r, err := strconv.Atoi(regStr); err == nil && r > 0 {
			registerIntervalSec = r
		}
	}

	deviceToken := os.Getenv("DEVICE_TOKEN")
	controlToken := os.Getenv("CONTROL_TOKEN")

	hostname, _ := os.Hostname()
	advertiseURL := os.Getenv("ADVERTISE_URL")
	if advertiseURL == "" {
		advertiseURL = fmt.Sprintf("http://%s:%s", hostname, port)
	}
	advertiseURL = strings.TrimRight(advertiseURL, "/")

	// Validate Device Spec
	specRaw := loadSpecRaw()
	valRes := spec.ValidateSpec(specRaw)
	if !valRes.OK {
		for _, e := range valRes.Errors {
			logJSON("error", "invalid device spec", map[string]interface{}{
				"path":    e.Path,
				"message": e.Message,
			})
		}
		fatal(fmt.Sprintf("device spec has %d error(s), refusing to start", len(valRes.Errors)), nil)
	}

	// Initialize Device Engine
	dev := device.NewDevice(device.Config{
		DeviceID:  deviceID,
		Spec:      valRes.Spec,
		Seed:      seedPtr,
		TimeScale: timeScale,
	})

	// Initialize Gateway Client
	gwClient := gateway.NewClient(gateway.Config{
		GatewayURL:          gatewayURL,
		Device:              dev,
		Endpoint:            advertiseURL,
		Token:               deviceToken,
		RegisterIntervalSec: registerIntervalSec,
	})

	dev.OnEvent(func(evt device.Event) {
		gwClient.EnqueueEvent(evt)
		if evt.Type == "STATUS_CHANGED" && evt.Status == "ONLINE" {
			gwClient.Kick()
		}
	})

	// Initialize HTTP Server
	srvHandler := server.NewHandler(server.Config{
		Device:       dev,
		Client:       gwClient,
		DeviceToken:  deviceToken,
		ControlToken: controlToken,
	})

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: srvHandler,
	}

	// Start Simulation Loop (every 250ms)
	stopLoop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		last := time.Now()
		for {
			select {
			case <-stopLoop:
				return
			case now := <-ticker.C:
				dt := now.Sub(last).Seconds()
				last = now
				readings := dev.Tick(dt)
				gwClient.EnqueueTelemetry(readings)
			}
		}
	}()

	// Start HTTP Server
	go func() {
		controlState := "disabled"
		if controlToken != "" {
			controlState = "enabled"
		}
		authState := "none"
		if deviceToken != "" {
			authState = "token"
		}

		logJSON("info", "device runtime started", map[string]interface{}{
			"deviceId":  deviceID,
			"type":      valRes.Spec.Type,
			"version":   valRes.Spec.Version,
			"port":      port,
			"endpoint":  advertiseURL,
			"gateway":   gatewayURL,
			"timeScale": timeScale,
			"auth":      authState,
			"control":   controlState,
		})

		gwClient.Start()

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logJSON("info", "shutting down", nil)
	close(stopLoop)
	gwClient.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
