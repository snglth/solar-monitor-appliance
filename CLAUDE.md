# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A NixOS flake that builds an SD card image for a Raspberry Pi 4B running a solar IoT monitoring stack. The target architecture is `aarch64-linux`.

## Build Commands

```bash
# Build the SD image (cross-compiles on non-aarch64 hosts)
nix build .#nixosConfigurations.solar-monitor.config.system.build.sdImage

# Evaluate a config value (useful for checking option resolution)
nix eval .#nixosConfigurations.solar-monitor.config.<attr-path>

# Update flake inputs
nix flake update
```

The resulting image lands in `result/sd-image/`. It is zstd-compressed via `postBuildCommands` (`sdImage.compressImage` is `false` to avoid a double-compress cycle).

## Architecture

The flake produces a single NixOS configuration (`solar-monitor`) composed of:

- **`flake.nix`** — Top-level system config. Defines the full service stack inline: Mosquitto (MQTT broker, port 1883), mqtt-bridge (MQTT→VictoriaMetrics bridge), VictoriaMetrics (port 8428), and Grafana (port 3000). Also configures networking (iwd for WiFi, DHCP on wlan0), firewall, SSH (key-only), and system packages.

- **`modules/user-config.nix`** — First-boot user customization module. Reads `/boot/firmware/config.json` from the SD card's firmware partition and applies WiFi credentials, SSH keys, timezone, and MQTT credentials. The actual logic lives in a Go binary (`cmd/apply-user-config/`). A default `config.json` skeleton is seeded onto the firmware partition at build time. Runs as a systemd oneshot before all services it configures.

### Data Flow

```
IoT sensors → MQTT (Mosquitto :1883) → mqtt-bridge → VictoriaMetrics (:8428) → Grafana (:3000)
```

A lightweight Go binary (`cmd/mqtt-bridge/`) subscribes to all MQTT topics (`#`) via the Eclipse Paho client, parses plain numeric payloads, converts them to InfluxDB line protocol, and writes to VictoriaMetrics via the `/write` endpoint. It reads the MQTT password from the `MQTT_PASSWORD` environment variable (loaded via `EnvironmentFile`). Grafana queries VictoriaMetrics using PromQL via the built-in Prometheus datasource. Power metrics (`solar_pv_power`, `solar_load_power`) are expected from the MCU but the dashboard computes `V × I` server-side as a fallback.

### Secrets / Credentials

- **MQTT password file**: `/var/lib/mosquitto/monitor-password` (plain-text, used by Mosquitto's `passwordFile` option). Defaults to `monitor`; overridden via `config.json`.
- **Grafana secret key**: Auto-generated on first boot via `openssl rand`.

### Service Ordering

`apply-user-config` runs after `local-fs.target` and before `iwd`, `sshd`, `mosquitto`, `mqtt-bridge`, `grafana`. `mqtt-bridge` explicitly depends on both Mosquitto and VictoriaMetrics.
