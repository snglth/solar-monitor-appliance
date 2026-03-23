#pragma once

// ── WiFi ────────────────────────────────────────────────────────────
#ifndef WIFI_SSID
#define WIFI_SSID "YOUR_SSID"
#endif

#ifndef WIFI_PASSWORD
#define WIFI_PASSWORD "YOUR_PASSWORD"
#endif

// ── MQTT ────────────────────────────────────────────────────────────
#ifndef MQTT_USER
#define MQTT_USER "monitor"
#endif

#ifndef MQTT_PASSWORD
#define MQTT_PASSWORD "monitor"
#endif

#ifndef MQTT_HOST
#define MQTT_HOST "solar-monitor"  // mDNS hostname (without .local)
#endif

#ifndef MQTT_PORT
#define MQTT_PORT 1883
#endif

// ── Simulation ──────────────────────────────────────────────────────
#ifndef PUBLISH_INTERVAL_MS
#define PUBLISH_INTERVAL_MS 5000  // 5 seconds between publishes
#endif

#ifndef SIM_DAY_SECONDS
#define SIM_DAY_SECONDS 600  // one simulated day = 10 real minutes
#endif
