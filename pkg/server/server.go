package server

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"fogsense-device-runtime/pkg/device"
	"fogsense-device-runtime/pkg/gateway"
)

const maxBodyBytes = 64 * 1024

type Config struct {
	Device       *device.Device
	Client       *gateway.Client
	DeviceToken  string
	ControlToken string
}

func safeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
}

func NewHandler(cfg Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Health endpoint (public)
		if r.Method == http.MethodGet && path == "/health" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"status":     "UP",
				"deviceId":   cfg.Device.DeviceID,
				"bootId":     cfg.Device.BootID,
				"registered": cfg.Client.IsRegistered(),
			})
			return
		}

		// Instructor Control API
		if strings.HasPrefix(path, "/control/") {
			if cfg.ControlToken == "" {
				writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
				return
			}
			token := r.Header.Get("X-Control-Token")
			if !safeEqual(token, cfg.ControlToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
				return
			}

			if r.Method == http.MethodGet && path == "/control/status" {
				res := cfg.Device.ControlStatus()
				res["gateway"] = cfg.Client.Stats()
				writeJSON(w, http.StatusOK, res)
				return
			}

			if r.Method == http.MethodPost && path == "/control/fault" {
				body, err := readBody(r)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid body"})
					return
				}
				status, respBody := cfg.Device.ApplyFault(body)
				writeJSON(w, status, respBody)
				return
			}

			writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
			return
		}

		// Device API authentication
		if cfg.DeviceToken != "" {
			token := r.Header.Get("X-Device-Token")
			if !safeEqual(token, cfg.DeviceToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "unauthorized"})
				return
			}
		}

		// Device API endpoints
		if r.Method == http.MethodGet && path == "/info" {
			writeJSON(w, http.StatusOK, cfg.Device.Info())
			return
		}

		if r.Method == http.MethodGet && path == "/state" {
			writeJSON(w, http.StatusOK, cfg.Device.StateView())
			return
		}

		if r.Method == http.MethodPost && path == "/commands" {
			body, err := readBody(r)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{
					"status":  "REJECTED",
					"reason":  "INVALID_REQUEST",
					"message": "failed to read body",
				})
				return
			}
			status, respBody := cfg.Device.ExecuteCommand(body)
			writeJSON(w, status, respBody)
			return
		}

		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
	})

	return mux
}
