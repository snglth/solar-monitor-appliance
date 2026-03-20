#!/usr/bin/env bash
set -euo pipefail

HOST="${1:-10.44.0.1}"
USER="monitor"
PASS="monitor"

while true; do
  voltage=$(awk "BEGIN {printf \"%.1f\", 48 + rand() * 4}")
  current=$(awk "BEGIN {printf \"%.2f\", 5 + rand() * 3}")
  power=$(awk "BEGIN {printf \"%.1f\", $voltage * $current}")
  temp=$(awk "BEGIN {printf \"%.1f\", 25 + rand() * 15}")
  soc=$(awk "BEGIN {printf \"%.1f\", 60 + rand() * 40}")

  mosquitto_pub -h "$HOST" -u "$USER" -P "$PASS" -t "solar/pv/voltage" -m "$voltage"
  mosquitto_pub -h "$HOST" -u "$USER" -P "$PASS" -t "solar/pv/current" -m "$current"
  mosquitto_pub -h "$HOST" -u "$USER" -P "$PASS" -t "solar/pv/power" -m "$power"
  mosquitto_pub -h "$HOST" -u "$USER" -P "$PASS" -t "solar/battery/temperature" -m "$temp"
  mosquitto_pub -h "$HOST" -u "$USER" -P "$PASS" -t "solar/battery/soc" -m "$soc"

  echo "Published: voltage=$voltage current=$current power=$power temp=$temp soc=$soc"
  sleep 5
done
