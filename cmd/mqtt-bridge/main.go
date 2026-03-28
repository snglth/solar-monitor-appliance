package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var version = "dev"

var (
	forwarded atomic.Int64
	skipped   atomic.Int64
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

	log.Printf("mqtt-bridge %s starting", version)
	log.Printf("mqtt broker: %s  user: %s", *mqttAddr, *mqttUser)
	log.Printf("victoriametrics: %s", *vmURL)

	httpClient := &http.Client{Timeout: 5 * time.Second}

	opts := mqtt.NewClientOptions().
		AddBroker("tcp://"+*mqttAddr).
		SetClientID("mqtt-bridge").
		SetUsername(*mqttUser).
		SetPassword(mqttPass).
		SetKeepAlive(30*time.Second).
		SetCleanSession(true).
		SetOrderMatters(false).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Printf("connected to %s", *mqttAddr)
			token := c.Subscribe("#", 0, makeHandler(httpClient, *vmURL))
			if token.Wait() && token.Error() != nil {
				log.Printf("subscribe error: %v", token.Error())
			} else {
				log.Printf("subscribed to all topics (#)")
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Printf("connection lost: %v", err)
		}).
		SetReconnectingHandler(func(_ mqtt.Client, opts *mqtt.ClientOptions) {
			log.Printf("reconnecting to %s...", *mqttAddr)
		})

	c := mqtt.NewClient(opts)
	token := c.Connect()
	token.Wait()

	// Start periodic stats logger
	go logStats()

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	s := <-sig
	log.Printf("received %s, shutting down", s)
	c.Disconnect(1000)
	logStatLine()
}

func logStats() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		logStatLine()
	}
}

func logStatLine() {
	f := forwarded.Swap(0)
	s := skipped.Swap(0)
	log.Printf("stats: forwarded=%d skipped=%d (last 60s)", f, s)
}

func makeHandler(client *http.Client, vmURL string) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()
		payload := string(msg.Payload())

		line, reason := toLineProtocol(topic, payload)
		if line == "" {
			skipped.Add(1)
			log.Printf("skip: topic=%q payload=%q reason=%s", topic, payload, reason)
			return
		}

		resp, err := client.Post(vmURL, "text/plain", bytes.NewBufferString(line))
		if err != nil {
			skipped.Add(1)
			log.Printf("POST error: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			skipped.Add(1)
			log.Printf("POST %d: %s", resp.StatusCode, bytes.TrimSpace(body))
			return
		}

		forwarded.Add(1)
	}
}

// toLineProtocol converts topic "solar/pv/voltage" and payload "24.5" to
// InfluxDB line protocol: "solar_pv,mqtt_topic=solar/pv/voltage voltage=24.5".
// Returns ("", reason) on skip.
func toLineProtocol(topic, payload string) (string, string) {
	payload = strings.TrimSpace(payload)
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return "", "topic has fewer than 3 parts"
	}
	value, err := strconv.ParseFloat(payload, 64)
	if err != nil {
		return "", fmt.Sprintf("non-numeric payload: %v", err)
	}
	field := parts[len(parts)-1]
	measurement := strings.Join(parts[:len(parts)-1], "_")
	line := fmt.Sprintf("%s,mqtt_topic=%s %s=%s",
		measurement, topic, field, strconv.FormatFloat(value, 'f', -1, 64))
	return line, ""
}
