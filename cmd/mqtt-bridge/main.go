package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	mqttAddr := flag.String("mqtt-addr", "127.0.0.1:1883", "MQTT broker address")
	mqttUser := flag.String("mqtt-user", "monitor", "MQTT username")
	vmURL := flag.String("vm-url", "http://127.0.0.1:8428/write", "VictoriaMetrics write URL")
	flag.Parse()

	mqttPass := os.Getenv("MQTT_PASSWORD")
	if mqttPass == "" {
		log.Fatal("MQTT_PASSWORD environment variable is required")
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}

	opts := mqtt.NewClientOptions().
		AddBroker("tcp://" + *mqttAddr).
		SetClientID("mqtt-bridge").
		SetUsername(*mqttUser).
		SetPassword(mqttPass).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Printf("connected to %s", *mqttAddr)
			if token := c.Subscribe("#", 0, makeHandler(httpClient, *vmURL)); token.Wait() && token.Error() != nil {
				log.Printf("subscribe error: %v", token.Error())
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Printf("connection lost: %v", err)
		})

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("connect: %v", token.Error())
	}

	select {} // block forever; paho handles reconnection
}

func makeHandler(client *http.Client, vmURL string) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		line := toLineProtocol(msg.Topic(), string(msg.Payload()))
		if line == "" {
			return
		}
		resp, err := client.Post(vmURL, "text/plain", bytes.NewBufferString(line))
		if err != nil {
			log.Printf("POST error: %v", err)
			return
		}
		resp.Body.Close()
	}
}

// toLineProtocol converts topic "solar/pv/voltage" and payload "24.5" to
// InfluxDB line protocol: "solar_pv,mqtt_topic=solar/pv/voltage voltage=24.5".
func toLineProtocol(topic, payload string) string {
	payload = strings.TrimSpace(payload)
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return ""
	}
	value, err := strconv.ParseFloat(payload, 64)
	if err != nil {
		return ""
	}
	field := parts[len(parts)-1]
	measurement := strings.Join(parts[:len(parts)-1], "_")
	return fmt.Sprintf("%s,mqtt_topic=%s %s=%s",
		measurement, topic, field, strconv.FormatFloat(value, 'f', -1, 64))
}
