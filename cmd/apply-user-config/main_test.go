package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── loadConfig tests ────────────────────────────────────────────────

func TestLoadConfig_Full(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"timezone": "America/Chicago",
		"wifi": {"ssid": "MyNet", "password": "secret"},
		"ssh_authorized_keys": ["ssh-ed25519 AAAA test@host"],
		"mqtt": {"password": "mqttpass"}
	}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timezone != "America/Chicago" {
		t.Errorf("timezone = %q, want %q", cfg.Timezone, "America/Chicago")
	}
	if cfg.WiFi.SSID != "MyNet" {
		t.Errorf("wifi.ssid = %q, want %q", cfg.WiFi.SSID, "MyNet")
	}
	if cfg.WiFi.Password != "secret" {
		t.Errorf("wifi.password = %q, want %q", cfg.WiFi.Password, "secret")
	}
	if len(cfg.SSHAuthorizedKeys) != 1 || cfg.SSHAuthorizedKeys[0] != "ssh-ed25519 AAAA test@host" {
		t.Errorf("ssh_authorized_keys = %v, want [ssh-ed25519 AAAA test@host]", cfg.SSHAuthorizedKeys)
	}
	if cfg.MQTT.Password != "mqttpass" {
		t.Errorf("mqtt.password = %q, want %q", cfg.MQTT.Password, "mqttpass")
	}
}

func TestLoadConfig_Minimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"mqtt":{"password":"onlythis"}}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WiFi.SSID != "" {
		t.Errorf("wifi.ssid = %q, want empty", cfg.WiFi.SSID)
	}
	if cfg.MQTT.Password != "onlythis" {
		t.Errorf("mqtt.password = %q, want %q", cfg.MQTT.Password, "onlythis")
	}
}

func TestLoadConfig_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timezone != "" || cfg.WiFi.SSID != "" || cfg.MQTT.Password != "" {
		t.Errorf("expected all zero values, got %+v", cfg)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{not json`), 0644)

	cfg, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg, err := loadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if cfg != nil {
		t.Errorf("expected nil config, got %+v", cfg)
	}
}

// ── configureWiFi tests ────────────────────────────────────────────

func TestConfigureWiFi(t *testing.T) {
	dir := t.TempDir()
	iwdDir := filepath.Join(dir, "iwd")

	err := configureWiFi(iwdDir, "MyNet", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(iwdDir, "MyNet.psk")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read psk file: %v", err)
	}

	expected := "[Security]\nPassphrase=secret\n"
	if string(data) != expected {
		t.Errorf("psk content = %q, want %q", string(data), expected)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("psk file mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestConfigureWiFi_SpacesInSSID(t *testing.T) {
	dir := t.TempDir()
	iwdDir := filepath.Join(dir, "iwd")

	err := configureWiFi(iwdDir, "My Cool Net", "pass123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(iwdDir, "My Cool Net.psk")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file with spaces in name to exist: %v", err)
	}
}

func TestConfigureWiFi_EmptySSID(t *testing.T) {
	dir := t.TempDir()
	iwdDir := filepath.Join(dir, "iwd")

	err := configureWiFi(iwdDir, "", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No file should be created
	entries, _ := os.ReadDir(iwdDir)
	if len(entries) > 0 {
		t.Errorf("expected no files created, got %d", len(entries))
	}
}

func TestConfigureWiFi_EmptyPassword(t *testing.T) {
	dir := t.TempDir()
	iwdDir := filepath.Join(dir, "iwd")

	err := configureWiFi(iwdDir, "MyNet", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(iwdDir)
	if len(entries) > 0 {
		t.Errorf("expected no files created, got %d", len(entries))
	}
}

// ── configureSSHKeys tests ──────────────────────────────────────────

func TestConfigureSSHKeys_Fresh(t *testing.T) {
	dir := t.TempDir()
	authFile := filepath.Join(dir, ".ssh", "authorized_keys")

	keys := []string{"ssh-ed25519 AAAA key1", "ssh-rsa BBBB key2"}
	err := configureSSHKeys(authFile, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(authFile)
	content := string(data)

	if !strings.Contains(content, markerBegin) {
		t.Error("missing begin marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("missing end marker")
	}
	if !strings.Contains(content, "ssh-ed25519 AAAA key1") {
		t.Error("missing key1")
	}
	if !strings.Contains(content, "ssh-rsa BBBB key2") {
		t.Error("missing key2")
	}

	info, _ := os.Stat(authFile)
	if info.Mode().Perm() != 0600 {
		t.Errorf("file mode = %o, want 0600", info.Mode().Perm())
	}

	sshDir := filepath.Dir(authFile)
	dirInfo, _ := os.Stat(sshDir)
	if dirInfo.Mode().Perm() != 0700 {
		t.Errorf("dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}
}

func TestConfigureSSHKeys_PreservesUnmanaged(t *testing.T) {
	dir := t.TempDir()
	sshDir := filepath.Join(dir, ".ssh")
	os.MkdirAll(sshDir, 0700)
	authFile := filepath.Join(sshDir, "authorized_keys")

	// Write an existing unmanaged key
	os.WriteFile(authFile, []byte("ssh-rsa EXISTING manual@host\n"), 0600)

	keys := []string{"ssh-ed25519 AAAA managed@host"}
	err := configureSSHKeys(authFile, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(authFile)
	content := string(data)

	if !strings.Contains(content, "ssh-rsa EXISTING manual@host") {
		t.Error("unmanaged key was lost")
	}
	if !strings.Contains(content, "ssh-ed25519 AAAA managed@host") {
		t.Error("managed key was not added")
	}
}

func TestConfigureSSHKeys_Idempotent(t *testing.T) {
	dir := t.TempDir()
	sshDir := filepath.Join(dir, ".ssh")
	os.MkdirAll(sshDir, 0700)
	authFile := filepath.Join(sshDir, "authorized_keys")

	// Write an existing unmanaged key
	os.WriteFile(authFile, []byte("ssh-rsa EXISTING manual@host\n"), 0600)

	// First run
	keys1 := []string{"ssh-ed25519 AAAA first@host"}
	configureSSHKeys(authFile, keys1)

	// Second run with different keys
	keys2 := []string{"ssh-ed25519 BBBB second@host"}
	err := configureSSHKeys(authFile, keys2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(authFile)
	content := string(data)

	if !strings.Contains(content, "ssh-rsa EXISTING manual@host") {
		t.Error("unmanaged key was lost after second run")
	}
	if strings.Contains(content, "first@host") {
		t.Error("old managed key should have been replaced")
	}
	if !strings.Contains(content, "ssh-ed25519 BBBB second@host") {
		t.Error("new managed key not present")
	}

	// Should have exactly one begin marker
	if strings.Count(content, markerBegin) != 1 {
		t.Errorf("expected exactly 1 begin marker, got %d", strings.Count(content, markerBegin))
	}
}

func TestConfigureSSHKeys_EmptyKeys(t *testing.T) {
	dir := t.TempDir()
	authFile := filepath.Join(dir, "authorized_keys")
	os.WriteFile(authFile, []byte("existing\n"), 0600)

	err := configureSSHKeys(authFile, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should be unchanged
	data, _ := os.ReadFile(authFile)
	if string(data) != "existing\n" {
		t.Errorf("file was modified when keys were empty")
	}
}

// ── configureMQTT tests ────────────────────────────────────────────

func TestConfigureMQTT(t *testing.T) {
	dir := t.TempDir()
	passFile := filepath.Join(dir, "mosquitto", "monitor-password")
	envFile := filepath.Join(dir, "credentials", "mqtt.env")

	err := configureMQTT(passFile, envFile, "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Password file: no trailing newline
	passData, _ := os.ReadFile(passFile)
	if string(passData) != "secret" {
		t.Errorf("password file = %q, want %q", string(passData), "secret")
	}

	passInfo, _ := os.Stat(passFile)
	if passInfo.Mode().Perm() != 0600 {
		t.Errorf("password file mode = %o, want 0600", passInfo.Mode().Perm())
	}

	// Env file: with trailing newline
	envData, _ := os.ReadFile(envFile)
	if string(envData) != "MQTT_PASSWORD=secret\n" {
		t.Errorf("env file = %q, want %q", string(envData), "MQTT_PASSWORD=secret\n")
	}

	envInfo, _ := os.Stat(envFile)
	if envInfo.Mode().Perm() != 0600 {
		t.Errorf("env file mode = %o, want 0600", envInfo.Mode().Perm())
	}
}

func TestConfigureMQTT_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	passFile := filepath.Join(dir, "deep", "nested", "pass")
	envFile := filepath.Join(dir, "also", "nested", "env")

	err := configureMQTT(passFile, envFile, "pw")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(passFile); err != nil {
		t.Errorf("password file not created: %v", err)
	}
	if _, err := os.Stat(envFile); err != nil {
		t.Errorf("env file not created: %v", err)
	}
}

// ── setTimezone tests ───────────────────────────────────────────────

func TestSetTimezone(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("skipping on unsupported OS")
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "args")

	mockBin := filepath.Join(dir, "mock-timedatectl")
	script := "#!/bin/sh\necho \"$@\" > " + outFile + "\n"
	os.WriteFile(mockBin, []byte(script), 0755)

	err := setTimezone(mockBin, "America/Chicago")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(outFile)
	got := strings.TrimSpace(string(data))
	if got != "set-timezone America/Chicago" {
		t.Errorf("args = %q, want %q", got, "set-timezone America/Chicago")
	}
}

func TestSetTimezone_Empty(t *testing.T) {
	err := setTimezone("/nonexistent", "")
	if err != nil {
		t.Errorf("expected nil for empty timezone, got %v", err)
	}
}

// ── configureMAC tests ──────────────────────────────────────────────

func TestConfigureMAC(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("skipping on unsupported OS")
	}

	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls")

	mockBin := filepath.Join(dir, "mock-ip")
	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\n"
	os.WriteFile(mockBin, []byte(script), 0755)

	err := configureMAC(mockBin, "wlan0", "02:42:ac:11:00:02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(logFile)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 ip invocations, got %d: %v", len(lines), lines)
	}
	if lines[0] != "link set wlan0 down" {
		t.Errorf("call 1 = %q, want %q", lines[0], "link set wlan0 down")
	}
	if lines[1] != "link set wlan0 address 02:42:ac:11:00:02" {
		t.Errorf("call 2 = %q, want %q", lines[1], "link set wlan0 address 02:42:ac:11:00:02")
	}
	if lines[2] != "link set wlan0 up" {
		t.Errorf("call 3 = %q, want %q", lines[2], "link set wlan0 up")
	}
}

func TestConfigureMAC_Empty(t *testing.T) {
	err := configureMAC("/nonexistent/ip", "wlan0", "")
	if err != nil {
		t.Errorf("expected nil for empty MAC, got %v", err)
	}
}

func TestConfigureMAC_InvalidMAC(t *testing.T) {
	err := configureMAC("/nonexistent/ip", "wlan0", "not-a-mac")
	if err == nil {
		t.Fatal("expected error for invalid MAC address")
	}
	if !strings.Contains(err.Error(), "invalid MAC address") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "invalid MAC address")
	}
}

func TestLoadConfig_WithMACAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"wifi": {"ssid": "Net", "password": "pw", "mac_address": "02:42:ac:11:00:02"}
	}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WiFi.MACAddress != "02:42:ac:11:00:02" {
		t.Errorf("mac_address = %q, want %q", cfg.WiFi.MACAddress, "02:42:ac:11:00:02")
	}
}

// ── Integration-style: verify JSON round-trip ───────────────────────

func TestConfigRoundTrip(t *testing.T) {
	cfg := Config{
		Timezone: "UTC",
	}
	cfg.WiFi.SSID = "Net"
	cfg.WiFi.Password = "pass"
	cfg.SSHAuthorizedKeys = []string{"key1"}
	cfg.MQTT.Password = "mqtt"

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, data, 0644)

	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.WiFi.SSID != cfg.WiFi.SSID || loaded.MQTT.Password != cfg.MQTT.Password {
		t.Errorf("round-trip mismatch: got %+v", loaded)
	}
}
