#include <Arduino.h>
#include <WiFi.h>
#include <ESPmDNS.h>
#include <PubSubClient.h>

#include "config.h"
#include "solar_sim.h"

// ── Globals ─────────────────────────────────────────────────────────
WiFiClient   wifi_client;
PubSubClient mqtt(wifi_client);
SolarSim     sim;
IPAddress    broker_ip;

static unsigned long last_publish = 0;

// ── WiFi ────────────────────────────────────────────────────────────

void setup_wifi() {
    Serial.printf("[WiFi] Connecting to %s", WIFI_SSID);
    WiFi.mode(WIFI_STA);
    WiFi.setAutoReconnect(true);
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        Serial.printf("\n[WiFi] Connected, IP: %s\n", WiFi.localIP().toString().c_str());
    } else {
        Serial.println("\n[WiFi] Connection failed, will retry in loop");
    }
}

bool ensure_wifi() {
    if (WiFi.status() == WL_CONNECTED)
        return true;

    Serial.println("[WiFi] Disconnected, reconnecting...");
    WiFi.disconnect();
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        Serial.printf("[WiFi] Reconnected, IP: %s\n", WiFi.localIP().toString().c_str());
        return true;
    }
    return false;
}

// ── mDNS ────────────────────────────────────────────────────────────

bool resolve_broker() {
    Serial.printf("[mDNS] Resolving %s.local...\n", MQTT_HOST);

    for (int i = 0; i < 5; i++) {
        broker_ip = MDNS.queryHost(MQTT_HOST, 5000);
        if (broker_ip != IPAddress(0, 0, 0, 0)) {
            Serial.printf("[mDNS] Resolved: %s\n", broker_ip.toString().c_str());
            return true;
        }
        Serial.printf("[mDNS] Attempt %d failed, retrying...\n", i + 1);
        delay(2000);
    }

    Serial.println("[mDNS] Resolution failed");
    return false;
}

// ── MQTT ────────────────────────────────────────────────────────────

bool ensure_mqtt() {
    if (mqtt.connected())
        return true;

    if (!resolve_broker())
        return false;

    mqtt.setServer(broker_ip, MQTT_PORT);
    Serial.printf("[MQTT] Connecting to %s:%d...\n", broker_ip.toString().c_str(), MQTT_PORT);

    if (mqtt.connect("solar-sensor", MQTT_USER, MQTT_PASSWORD)) {
        Serial.println("[MQTT] Connected");
        return true;
    }

    Serial.printf("[MQTT] Failed, rc=%d\n", mqtt.state());
    return false;
}

// ── Publish ─────────────────────────────────────────────────────────

void publish_readings(const SolarReadings& r) {
    char buf[16];

    dtostrf(r.pv_voltage, 1, 2, buf);
    mqtt.publish("solar/pv/voltage", buf);

    dtostrf(r.pv_current, 1, 3, buf);
    mqtt.publish("solar/pv/current", buf);

    dtostrf(r.pv_power, 1, 2, buf);
    mqtt.publish("solar/pv/power", buf);

    dtostrf(r.load_voltage, 1, 2, buf);
    mqtt.publish("solar/load/voltage", buf);

    dtostrf(r.load_current, 1, 3, buf);
    mqtt.publish("solar/load/current", buf);

    dtostrf(r.load_power, 1, 2, buf);
    mqtt.publish("solar/load/power", buf);

    dtostrf(r.battery_voltage, 1, 2, buf);
    mqtt.publish("solar/battery/voltage", buf);

    Serial.printf("sim %05.2fh  PV %4.1fV/%4.2fA  Bat %5.2fV  Load %4.2fA\n",
                  r.sim_hour, r.pv_voltage, r.pv_current,
                  r.battery_voltage, r.load_current);
}

// ── Arduino entry points ────────────────────────────────────────────

void setup() {
    Serial.begin(115200);
    delay(500);
    Serial.println("\n== solar-sensor demo ==");

    setup_wifi();

    if (!MDNS.begin("solar-sensor")) {
        Serial.println("[mDNS] Init failed");
    }

    if (WiFi.status() == WL_CONNECTED) {
        resolve_broker();
        mqtt.setServer(broker_ip, MQTT_PORT);
        ensure_mqtt();
    }
}

void loop() {
    if (!ensure_wifi())
        return;

    if (!ensure_mqtt())
        return;

    mqtt.loop();

    unsigned long now = millis();
    if (now - last_publish >= PUBLISH_INTERVAL_MS) {
        last_publish = now;
        SolarReadings r = sim.update((float)PUBLISH_INTERVAL_MS / 1000.0f);
        publish_readings(r);
    }
}
