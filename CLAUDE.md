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

The resulting image lands in `result/sd-image/`. It is zstd-compressed (`sdImage.compressImage = true`).

## Architecture

The flake produces a single NixOS configuration (`solar-monitor`) composed of:

- **`flake.nix`** — Top-level system config. Defines the full service stack inline: Mosquitto (MQTT broker, port 1883), Vector (MQTT→VictoriaMetrics bridge), VictoriaMetrics (port 8428), and Grafana (port 3000). Also configures networking (iwd for WiFi, DHCP on wlan0), firewall, SSH (key-only), and system packages.

- **`modules/user-config.nix`** — First-boot user customization module. Reads `/boot/firmware/config.json` from the SD card's firmware partition and applies WiFi credentials, SSH keys, hostname, timezone, and MQTT credentials. A default `config.json` skeleton is seeded onto the firmware partition at build time. Runs as a systemd oneshot before all services it configures.

### Data Flow

```
IoT sensors → MQTT (Mosquitto :1883) → Vector → VictoriaMetrics (:8428) → Grafana (:3000)
```

Vector subscribes to all MQTT topics (`#`), parses JSON payloads, converts numeric fields to InfluxDB line protocol, and writes to VictoriaMetrics via the `/write` endpoint. Grafana queries VictoriaMetrics using PromQL via the built-in Prometheus datasource.

### Secrets / Credentials

- **MQTT password file**: `/var/lib/mosquitto/passwd`. Seeded with `monitor:monitor` by a oneshot service; overridden via `config.json`.
- **Grafana secret key**: Auto-generated on first boot via `openssl rand`.

### Service Ordering

`apply-user-config` runs after `local-fs.target` and before `iwd`, `sshd`, `mosquitto`, `vector`, `grafana`. Vector explicitly depends on both Mosquitto and VictoriaMetrics.
