package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Hostname string `json:"hostname"`
	Timezone string `json:"timezone"`
	WiFi     struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	} `json:"wifi"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys"`
	MQTT              struct {
		Password string `json:"password"`
	} `json:"mqtt"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func configureWiFi(iwdDir, ssid, password string) error {
	if ssid == "" || password == "" {
		return nil
	}
	if err := os.MkdirAll(iwdDir, 0755); err != nil {
		return fmt.Errorf("creating iwd dir: %w", err)
	}
	content := fmt.Sprintf("[Security]\nPassphrase=%s\n", password)
	path := filepath.Join(iwdDir, ssid+".psk")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing WiFi config: %w", err)
	}
	return nil
}

const (
	markerBegin = "# BEGIN apply-user-config"
	markerEnd   = "# END apply-user-config"
)

func configureSSHKeys(authKeysFile string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	dir := filepath.Dir(authKeysFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating ssh dir: %w", err)
	}

	// Read existing content
	existing := ""
	if data, err := os.ReadFile(authKeysFile); err == nil {
		existing = string(data)
	}

	// Remove previous managed block if present
	if strings.Contains(existing, markerBegin) {
		before := existing[:strings.Index(existing, markerBegin)]
		afterIdx := strings.Index(existing, markerEnd)
		after := ""
		if afterIdx >= 0 {
			after = existing[afterIdx+len(markerEnd):]
			// Strip the newline immediately after the end marker
			if strings.HasPrefix(after, "\n") {
				after = after[1:]
			}
		}
		existing = before + after
	}

	// Build managed block
	block := markerBegin + "\n" + strings.Join(keys, "\n") + "\n" + markerEnd + "\n"

	content := existing + block

	if err := os.WriteFile(authKeysFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing authorized_keys: %w", err)
	}

	chownSSH(authKeysFile)
	return nil
}

func chownSSH(authKeysFile string) {
	u, err := user.Lookup("monitor")
	if err != nil {
		log.Printf("Warning: could not look up user 'monitor': %v", err)
		return
	}
	g, err := user.LookupGroup("users")
	if err != nil {
		log.Printf("Warning: could not look up group 'users': %v", err)
		return
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(g.Gid)

	dir := filepath.Dir(authKeysFile)
	if err := os.Chown(dir, uid, gid); err != nil {
		log.Printf("Warning: could not chown %s: %v", dir, err)
	}
	if err := os.Chown(authKeysFile, uid, gid); err != nil {
		log.Printf("Warning: could not chown %s: %v", authKeysFile, err)
	}
}

func setHostname(hostnamectlBin, hostname string) error {
	if hostname == "" {
		return nil
	}
	cmd := exec.Command(hostnamectlBin, "set-hostname", hostname)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func setTimezone(timedatectlBin, timezone string) error {
	if timezone == "" {
		return nil
	}
	cmd := exec.Command(timedatectlBin, "set-timezone", timezone)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func configureMQTT(passFile, envFile, password string) error {
	if err := os.MkdirAll(filepath.Dir(passFile), 0755); err != nil {
		return fmt.Errorf("creating password file dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(envFile), 0755); err != nil {
		return fmt.Errorf("creating env file dir: %w", err)
	}

	// Password file: no trailing newline
	if err := os.WriteFile(passFile, []byte(password), 0600); err != nil {
		return fmt.Errorf("writing password file: %w", err)
	}

	// Env file: with trailing newline
	envContent := fmt.Sprintf("MQTT_PASSWORD=%s\n", password)
	if err := os.WriteFile(envFile, []byte(envContent), 0600); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}

	return nil
}

func main() {
	configPath := flag.String("config", "/boot/firmware/config.json", "path to config.json")
	hostnamectlBin := flag.String("hostnamectl", "hostnamectl", "path to hostnamectl binary")
	timedatectlBin := flag.String("timedatectl", "timedatectl", "path to timedatectl binary")
	flag.Parse()

	mqttPass := "monitor"

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Printf("Could not load config (%v), using defaults for MQTT.", err)
	}

	if cfg != nil {
		log.Printf("Reading user config from %s", *configPath)

		if err := configureWiFi("/var/lib/iwd", cfg.WiFi.SSID, cfg.WiFi.Password); err != nil {
			log.Printf("Warning: WiFi configuration failed: %v", err)
		} else if cfg.WiFi.SSID != "" {
			log.Printf("Configured WiFi for SSID: %s", cfg.WiFi.SSID)
		}

		if err := configureSSHKeys("/home/monitor/.ssh/authorized_keys", cfg.SSHAuthorizedKeys); err != nil {
			log.Printf("Warning: SSH key configuration failed: %v", err)
		} else if len(cfg.SSHAuthorizedKeys) > 0 {
			log.Printf("Configured SSH authorized keys")
		}

		if err := setHostname(*hostnamectlBin, cfg.Hostname); err != nil {
			log.Printf("Warning: hostname configuration failed: %v", err)
		} else if cfg.Hostname != "" {
			log.Printf("Set hostname to: %s", cfg.Hostname)
		}

		if err := setTimezone(*timedatectlBin, cfg.Timezone); err != nil {
			log.Printf("Warning: timezone configuration failed: %v", err)
		} else if cfg.Timezone != "" {
			log.Printf("Set timezone to: %s", cfg.Timezone)
		}

		if cfg.MQTT.Password != "" {
			mqttPass = cfg.MQTT.Password
		}
	}

	log.Println("Configuring MQTT password")
	if err := configureMQTT("/var/lib/mosquitto/monitor-password", "/var/lib/vector/mqtt.env", mqttPass); err != nil {
		log.Fatalf("MQTT configuration failed: %v", err)
	}

	log.Println("User config applied successfully.")
}
