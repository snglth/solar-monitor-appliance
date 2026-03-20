#!/usr/bin/env python3
"""Generate 7 days of simulated solar monitor data in InfluxDB line protocol.

Reuses the solar simulation model from test-publish.py (irradiance curve,
battery state, load noise) but outputs InfluxDB line protocol to stdout
instead of publishing via MQTT.  Day-to-day variation is introduced by
randomizing cloud cover intensity and load baseline for each day.

Output matches the format Vector writes to VictoriaMetrics:
    solar_pv,mqtt_topic=solar/pv/voltage voltage=12.80 1710849600000
"""

import math
import random
import sys
import time
from datetime import datetime

# ── Hardware constants (same as test-publish.py) ─────────────────────
IMP = 1.71          # panel max-power current (A)
BATTERY_REST = 12.8 # fully-rested open-circuit voltage (V)
BATTERY_MIN = 11.5  # deep-discharge cutoff (V)
BATTERY_MAX = 14.4  # absorption charge voltage (V)
CAPACITY_AH = 18.0  # battery capacity (Ah)
INA_NOISE = 0.010   # INA219 measurement noise sigma (A)

# ── Generation parameters ────────────────────────────────────────────
DAYS = 7
INTERVAL = 5        # seconds between samples
SAMPLES_PER_DAY = 86400 // INTERVAL  # 17,280


def solar_irradiance(sim_hour: float, cloud_sigma: float) -> float:
    """Return 0-1 irradiance based on hour of day (0-24).

    Sunrise at 6, sunset at 18.  Smooth sine with random cloud noise
    controlled by cloud_sigma (varies per day).
    """
    if sim_hour < 6 or sim_hour > 18:
        return 0.0
    phase = (sim_hour - 6) / 12  # 0..1 over daylight window
    base = math.sin(phase * math.pi)
    return max(0.0, base + random.gauss(0, cloud_sigma))


def clamp(value: float, lo: float, hi: float) -> float:
    return max(lo, min(hi, value))


def main() -> None:
    now_s = int(time.time())
    start_s = now_s - DAYS * 86400

    # Seed for reproducibility (optional: remove for fully random runs)
    random.seed(42)

    battery_v = BATTERY_REST
    load_current = 1.0

    for day in range(DAYS):
        # Per-day variation
        cloud_sigma = random.uniform(0.02, 0.08)
        load_baseline = random.uniform(0.3, 0.6)
        load_current = load_baseline

        day_start_s = start_s + day * 86400

        for sample in range(SAMPLES_PER_DAY):
            ts_s = day_start_s + sample * INTERVAL
            ts_ms = ts_s * 1000
            local_dt = datetime.fromtimestamp(ts_s)
            sim_hour = local_dt.hour + local_dt.minute / 60 + local_dt.second / 3600

            irr = solar_irradiance(sim_hour, cloud_sigma)

            # ── PV ────────────────────────────────────────────────
            pv_current = IMP * irr + random.gauss(0, INA_NOISE)
            pv_current = max(0.0, pv_current)
            pv_voltage = battery_v if pv_current > 0.05 else 0.0
            pv_power = pv_voltage * pv_current

            # ── Load ──────────────────────────────────────────────
            load_current += random.gauss(0, 0.02)
            load_current = clamp(load_current, 0.15, 0.75)
            load_voltage = battery_v
            load_power = load_voltage * load_current

            # ── Battery ───────────────────────────────────────────
            net_current = pv_current - load_current
            dt_hours = INTERVAL / 3600.0
            dv = net_current * dt_hours / CAPACITY_AH * (BATTERY_MAX - BATTERY_MIN)
            battery_v += dv
            battery_v = clamp(battery_v, BATTERY_MIN, BATTERY_MAX)

            # ── Output InfluxDB line protocol ─────────────────────
            lines = (
                f"solar_pv,mqtt_topic=solar/pv/voltage voltage={pv_voltage:.2f} {ts_ms}",
                f"solar_pv,mqtt_topic=solar/pv/current current={pv_current:.3f} {ts_ms}",
                f"solar_pv,mqtt_topic=solar/pv/power power={pv_power:.2f} {ts_ms}",
                f"solar_load,mqtt_topic=solar/load/voltage voltage={load_voltage:.2f} {ts_ms}",
                f"solar_load,mqtt_topic=solar/load/current current={load_current:.3f} {ts_ms}",
                f"solar_load,mqtt_topic=solar/load/power power={load_power:.2f} {ts_ms}",
                f"solar_battery,mqtt_topic=solar/battery/voltage voltage={battery_v:.2f} {ts_ms}",
            )
            for line in lines:
                sys.stdout.write(line + "\n")

    sys.stderr.write(
        f"Generated {DAYS * SAMPLES_PER_DAY * 7} lines "
        f"({DAYS} days, {SAMPLES_PER_DAY} samples/day, 7 metrics)\n"
    )


if __name__ == "__main__":
    main()
