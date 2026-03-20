#!/usr/bin/env python3
"""Publish simulated solar monitor data over MQTT.

Emulates a 12 V off-grid system: 30 W polycrystalline panel (CL-SM30P),
PWM charge controller (KDY-20A), 12 V / 18 Ah AGM battery (UL18-12),
and a DC load — matching the INA219 sensor topics the Grafana dashboard
expects.

One simulated day completes in ~10 minutes so charging / discharging
cycles are visible on the dashboard almost immediately.
"""

import argparse
import math
import random
import time

import paho.mqtt.client as mqtt

# ── Hardware constants ──────────────────────────────────────────────
IMP = 1.71          # panel max-power current (A)
BATTERY_REST = 12.8 # fully-rested open-circuit voltage (V)
BATTERY_MIN = 11.5  # deep-discharge cutoff (V)
BATTERY_MAX = 14.4  # absorption charge voltage (V)
CAPACITY_AH = 18.0  # battery capacity (Ah)
LOAD_BASE_A = 1.0   # DC load baseline current (A)
INA_NOISE = 0.010   # INA219 measurement noise σ (A)

# ── Timing ──────────────────────────────────────────────────────────
PUBLISH_INTERVAL = 5            # seconds between publishes
SIM_DAY_SECONDS = 10 * 60      # real seconds per simulated day


def solar_irradiance(sim_hour: float) -> float:
    """Return 0–1 irradiance based on simulated hour (0–24).

    Sunrise at 6, sunset at 18.  Smooth sine with a small random
    perturbation to mimic passing clouds.
    """
    if sim_hour < 6 or sim_hour > 18:
        return 0.0
    phase = (sim_hour - 6) / 12  # 0..1 over daylight window
    base = math.sin(phase * math.pi)
    return max(0.0, base + random.gauss(0, 0.03))


def clamp(value: float, lo: float, hi: float) -> float:
    return max(lo, min(hi, value))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="10.44.0.1", help="MQTT broker host")
    parser.add_argument("--port", type=int, default=1883)
    parser.add_argument("--user", default="monitor")
    parser.add_argument("--password", default="monitor")
    args = parser.parse_args()

    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2)
    client.username_pw_set(args.user, args.password)
    client.connect(args.host, args.port)
    client.loop_start()

    # State
    battery_v = BATTERY_REST
    load_current = LOAD_BASE_A
    sim_time = 0.0  # seconds into simulated day

    try:
        while True:
            sim_hour = (sim_time % SIM_DAY_SECONDS) / SIM_DAY_SECONDS * 24.0
            irr = solar_irradiance(sim_hour)

            # ── PV ──────────────────────────────────────────────────
            pv_current = IMP * irr + random.gauss(0, INA_NOISE)
            pv_current = max(0.0, pv_current)
            # PWM controller clamps panel to battery voltage when generating
            pv_voltage = battery_v if pv_current > 0.05 else 0.0
            pv_power = pv_voltage * pv_current

            # ── Load ────────────────────────────────────────────────
            load_current += random.gauss(0, 0.02)
            load_current = clamp(load_current, 0.5, 1.5)
            load_voltage = battery_v
            load_power = load_voltage * load_current

            # ── Battery ─────────────────────────────────────────────
            net_current = pv_current - load_current  # positive = charging
            # ΔV ≈ net_current × Δt / capacity, scaled for voltage range
            dt_hours = PUBLISH_INTERVAL / 3600.0
            dv = net_current * dt_hours / CAPACITY_AH * (BATTERY_MAX - BATTERY_MIN)
            battery_v += dv
            battery_v = clamp(battery_v, BATTERY_MIN, BATTERY_MAX)

            # ── Publish ─────────────────────────────────────────────
            topics = {
                "solar/pv/voltage": f"{pv_voltage:.2f}",
                "solar/pv/current": f"{pv_current:.3f}",
                "solar/pv/power": f"{pv_power:.2f}",
                "solar/load/voltage": f"{load_voltage:.2f}",
                "solar/load/current": f"{load_current:.3f}",
                "solar/load/power": f"{load_power:.2f}",
                "solar/battery/voltage": f"{battery_v:.2f}",
            }
            for topic, value in topics.items():
                client.publish(topic, value)

            labels = (
                f"sim {sim_hour:05.2f}h  "
                f"PV {pv_voltage:.1f}V/{pv_current:.2f}A  "
                f"Bat {battery_v:.2f}V  "
                f"Load {load_current:.2f}A"
            )
            print(labels)

            sim_time += PUBLISH_INTERVAL
            time.sleep(PUBLISH_INTERVAL)
    except KeyboardInterrupt:
        pass
    finally:
        client.loop_stop()
        client.disconnect()


if __name__ == "__main__":
    main()
