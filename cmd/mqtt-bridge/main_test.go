package main

import "testing"

func TestToLineProtocol(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		payload string
		want    string
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
			name:    "too few topic parts",
			topic:   "solar/voltage",
			payload: "24.5",
			want:    "",
		},
		{
			name:    "non-numeric payload",
			topic:   "solar/pv/status",
			payload: "online",
			want:    "",
		},
		{
			name:    "empty payload",
			topic:   "solar/pv/voltage",
			payload: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLineProtocol(tt.topic, tt.payload)
			if got != tt.want {
				t.Errorf("toLineProtocol(%q, %q) = %q, want %q", tt.topic, tt.payload, got, tt.want)
			}
		})
	}
}
