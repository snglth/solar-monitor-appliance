# solar-monitor

NixOS SD card image for a Raspberry Pi 4B running a solar IoT monitoring stack.

```
IoT sensors → MQTT (Mosquitto :1883) → Vector → VictoriaMetrics (:8428) → Grafana (:3000)
```

## Preparing the SD card

Download the latest `solar-monitor-*.img.zst` from
[Releases](https://github.com/snglth/solar-monitor-appliance/releases/latest).

### macOS

```sh
diskutil list
diskutil unmountDisk /dev/diskN
zstdcat solar-monitor-*.img.zst | sudo dd of=/dev/rdiskN bs=4m status=progress
```

> Replace `/dev/diskN` with the correct disk number. Use `/dev/rdiskN` (raw disk) for faster writes.

### Linux

```sh
zstdcat solar-monitor-*.img.zst | sudo dd of=/dev/sdX bs=4M status=progress
```

> Replace `/dev/sdX` with the correct device (check with `lsblk`).

### Windows

1. Install [Raspberry Pi Imager](https://www.raspberrypi.com/software/).
2. Choose **Use custom** under Operating System and select the `.img.zst` file.
3. Choose your SD card and click **Write**.

## First-boot configuration

Mount the `FIRMWARE` partition after flashing and edit `config.json`:

```json
{
  "timezone": "Europe/Kyiv",
  "wifi": {
    "ssid": "YOUR_SSID",
    "password": "YOUR_WIFI_PASSWORD"
  },
  "ssh_authorized_keys": [
    "ssh-ed25519 AAAA..."
  ],
  "mqtt": {
    "password": "YOUR_MQTT_PASSWORD"
  }
}
```

All fields are optional. Defaults: timezone `UTC`, MQTT password `monitor`.

A systemd oneshot (`apply-user-config`) reads this file on every boot and configures WiFi (iwd), SSH keys, timezone, and MQTT credentials before the dependent services start.

Unmount, insert into the Pi, and boot.

## Networking

| Interface | Config | Address |
|-----------|--------|---------|
| `end0` (wired) | Static IP + DHCP server | `10.44.0.1/24` (clients get `.100`–`.199`) |
| `wlan0` (wireless) | DHCP client | assigned by your router |

Connect a computer directly via Ethernet to reach the Pi at `10.44.0.1`.

Firewall allows TCP ports 1883 (MQTT) and 3000 (Grafana). The wired interface (`end0`) is fully trusted.

## Services

| Service | Port | Notes |
|---------|------|-------|
| **Mosquitto** | 1883 | MQTT broker, password auth required |
| **VictoriaMetrics** | 8428 | Time-series DB, 12-month retention, internal only |
| **Vector** | — | Bridges MQTT → VictoriaMetrics via InfluxDB line protocol |
| **Grafana** | 3000 | Dashboards, auto-provisioned with VictoriaMetrics datasource |

### SSH

```sh
ssh monitor@10.44.0.1
```

Password authentication is enabled. Default password is set at build time. Add your SSH key via `config.json` for key-based access.

### Grafana

Open `http://10.44.0.1:3000` in a browser. Default login is `admin` / `admin`. A VictoriaMetrics datasource and a solar monitoring dashboard are pre-provisioned.

## MQTT API

The MQTT password set in `config.json` must match what the MCU uses to connect.

| Parameter | Value |
|-----------|-------|
| Host | `10.44.0.1` (wired) or Pi's WiFi IP |
| Port | `1883` |
| Username | `monitor` |
| Password | as set in `config.json` (default: `monitor`) |

### Topics

Publish each reading as a **plain numeric string** to a hierarchical topic:

```
solar/<source>/<field>  →  <float value>
```

Vector transforms topics into InfluxDB line protocol: `solar/pv/voltage` becomes measurement `solar_pv`, field `voltage`.

#### Required

| Topic | Unit | Source |
|-------|------|--------|
| `solar/pv/voltage` | V | PV INA219 |
| `solar/pv/current` | A | PV INA219 |
| `solar/pv/power` | W | computed on MCU |
| `solar/load/voltage` | V | Load INA219 |
| `solar/load/current` | A | Load INA219 |
| `solar/load/power` | W | computed on MCU |
| `solar/battery/voltage` | V | Battery INA219 |

#### Optional

| Topic | Unit | Note |
|-------|------|------|
| `solar/battery/current` | A | Can be omitted — server computes `Ipv - Iload` |

### Verify

```sh
mosquitto_sub -h 10.44.0.1 -u monitor -P monitor -t 'solar/#' -v
```

## Development

Requires [Nix](https://nixos.org/download/) with flakes enabled. [Garnix](https://garnix.io) is configured as a binary cache.

Build the SD image (requires an `aarch64-linux` builder or binfmt emulation):

```sh
nix build
```

The resulting image is at `result/sd-image/solar-monitor-*.img.zst`. Tagged commits also produce release images via GitHub Actions.

A dev shell with `mosquitto` CLI tools and `paho-mqtt` (Python) is provided:

```sh
nix develop
```
