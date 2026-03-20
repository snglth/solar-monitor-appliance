# MQTT API for Arduino/ESP

## Preparing the Raspberry Pi

Download the latest `solar-monitor-*.img.zst` from
[Releases](https://github.com/snglth/solar-monitor-appliance/releases/latest).

Flash to an SD card:

```sh
zstdcat solar-monitor-*.img.zst | sudo dd of=/dev/sdX bs=4M status=progress
```

Mount the `FIRMWARE` partition and edit `config.json`:

```
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

Unmount, insert into the Pi, boot. The MQTT password you set here must match what the MCU uses to connect.

## Broker

| Parameter | Value |
|-----------|-------|
| Host | Raspberry Pi IP |
| Port | `1883` |
| Username | `monitor` |
| Password | as set in `config.json` (default: `monitor`) |

## Topics

Publish each reading as a **plain numeric string** to a hierarchical topic:

```
solar/<source>/<field>  →  <float value>
```

### Required

| Topic | Unit | Source |
|-------|------|--------|
| `solar/pv/voltage` | V | PV INA219 |
| `solar/pv/current` | A | PV INA219 |
| `solar/pv/power` | W | computed on MCU |
| `solar/load/voltage` | V | Load INA219 |
| `solar/load/current` | A | Load INA219 |
| `solar/load/power` | W | computed on MCU |
| `solar/battery/voltage` | V | Battery INA219 |

### Optional

| Topic | Unit | Note |
|-------|------|------|
| `solar/battery/current` | A | Can be omitted — server computes `Ipv − Iload` |

## Verify from Raspberry Pi

```sh
mosquitto_sub -h localhost -u monitor -P monitor -t 'solar/#' -v
```
