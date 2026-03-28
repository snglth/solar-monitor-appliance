package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func TestToLineProtocol(t *testing.T) {
	tests := []struct {
		name       string
		topic      string
		payload    string
		want       string
		wantReason string
	}{
		{
			name:    "standard solar topic",
			topic:   "solar/pv/voltage",
			payload: "24.5",
			want:    "solar_pv,mqtt_topic=solar/pv/voltage voltage=24.5",
		},
		{
			name:    "integer value",
			topic:   "solar/load/power",
			payload: "150",
			want:    "solar_load,mqtt_topic=solar/load/power power=150",
		},
		{
			name:    "negative value",
			topic:   "solar/battery/current",
			payload: "-3.2",
			want:    "solar_battery,mqtt_topic=solar/battery/current current=-3.2",
		},
		{
			name:    "deep topic",
			topic:   "home/room/sensor/temperature",
			payload: "22.1",
			want:    "home_room_sensor,mqtt_topic=home/room/sensor/temperature temperature=22.1",
		},
		{
			name:    "payload with whitespace",
			topic:   "solar/pv/voltage",
			payload: " 24.5 ",
			want:    "solar_pv,mqtt_topic=solar/pv/voltage voltage=24.5",
		},
		{
			name:       "too few topic parts",
			topic:      "solar/voltage",
			payload:    "24.5",
			want:       "",
			wantReason: "topic has fewer than 3 parts",
		},
		{
			name:       "non-numeric payload",
			topic:      "solar/pv/status",
			payload:    "online",
			want:       "",
			wantReason: "non-numeric payload",
		},
		{
			name:       "empty payload",
			topic:      "solar/pv/voltage",
			payload:    "",
			want:       "",
			wantReason: "non-numeric payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := toLineProtocol(tt.topic, tt.payload)
			if got != tt.want {
				t.Errorf("toLineProtocol(%q, %q) line = %q, want %q", tt.topic, tt.payload, got, tt.want)
			}
			if tt.wantReason != "" && reason == "" {
				t.Errorf("toLineProtocol(%q, %q) expected reason containing %q, got empty", tt.topic, tt.payload, tt.wantReason)
			}
			if tt.wantReason != "" && reason != "" && !contains(reason, tt.wantReason) {
				t.Errorf("toLineProtocol(%q, %q) reason = %q, want containing %q", tt.topic, tt.payload, reason, tt.wantReason)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandlerForwardsToVM(t *testing.T) {
	var received []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		received = append(received, string(body[:n]))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	forwarded.Store(0)
	skipped.Store(0)

	handler := makeHandler(ts.Client(), ts.URL)
	handler(nil, &fakeMessage{topic: "solar/pv/voltage", payload: "24.5"})

	if len(received) != 1 {
		t.Fatalf("expected 1 request, got %d", len(received))
	}
	want := "solar_pv,mqtt_topic=solar/pv/voltage voltage=24.5"
	if received[0] != want {
		t.Errorf("body = %q, want %q", received[0], want)
	}
	if f := forwarded.Load(); f != 1 {
		t.Errorf("forwarded = %d, want 1", f)
	}
}

func TestHandlerLogsNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad line protocol"))
	}))
	defer ts.Close()

	forwarded.Store(0)
	skipped.Store(0)

	handler := makeHandler(ts.Client(), ts.URL)
	handler(nil, &fakeMessage{topic: "solar/pv/voltage", payload: "24.5"})

	if f := forwarded.Load(); f != 0 {
		t.Errorf("forwarded = %d, want 0", f)
	}
	if s := skipped.Load(); s != 1 {
		t.Errorf("skipped = %d, want 1", s)
	}
}

func TestHandlerSkipsInvalidMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not have made a request for invalid message")
	}))
	defer ts.Close()

	forwarded.Store(0)
	skipped.Store(0)

	handler := makeHandler(ts.Client(), ts.URL)
	handler(nil, &fakeMessage{topic: "solar/voltage", payload: "24.5"}) // too few parts

	if s := skipped.Load(); s != 1 {
		t.Errorf("skipped = %d, want 1", s)
	}
}

// fakeMessage implements mqtt.Message for testing.
type fakeMessage struct {
	topic   string
	payload string
}

func (m *fakeMessage) Duplicate() bool        { return false }
func (m *fakeMessage) Qos() byte              { return 0 }
func (m *fakeMessage) Retained() bool         { return false }
func (m *fakeMessage) Topic() string           { return m.topic }
func (m *fakeMessage) MessageID() uint16       { return 0 }
func (m *fakeMessage) Payload() []byte         { return []byte(m.payload) }
func (m *fakeMessage) Ack()                    {}

var _ mqtt.Message = (*fakeMessage)(nil)
