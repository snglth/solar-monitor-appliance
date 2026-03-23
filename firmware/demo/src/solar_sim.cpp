#include "solar_sim.h"
#include "config.h"
#include <math.h>

SolarSim::SolarSim()
    : battery_v(BATTERY_REST)
    , load_current(LOAD_BASE_A)
    , sim_time(0.0f)
{
    randomSeed(analogRead(0));
}

SolarReadings SolarSim::update(float dt_seconds) {
    float sim_day = (float)SIM_DAY_SECONDS;
    sim_time = fmod(sim_time + dt_seconds, sim_day);
    float hour = sim_time / sim_day * 24.0f;

    float irr = solar_irradiance(hour);

    // ── PV ──────────────────────────────────────────────────────────
    float pv_current = IMP * irr + gauss(0.0f, INA_NOISE);
    if (pv_current < 0.0f) pv_current = 0.0f;

    float pv_voltage = (pv_current > 0.05f) ? battery_v : 0.0f;
    float pv_power   = pv_voltage * pv_current;

    // ── Load ────────────────────────────────────────────────────────
    load_current += gauss(0.0f, 0.02f);
    load_current = clamp(load_current, 0.5f, 1.5f);

    float load_voltage = battery_v;
    float load_power   = load_voltage * load_current;

    // ── Battery ─────────────────────────────────────────────────────
    float net_current = pv_current - load_current;
    float dt_hours    = dt_seconds / 3600.0f;
    float dv = net_current * dt_hours / CAPACITY_AH * (BATTERY_MAX - BATTERY_MIN);
    battery_v += dv;
    battery_v = clamp(battery_v, BATTERY_MIN, BATTERY_MAX);

    return SolarReadings{
        pv_voltage,
        pv_current,
        pv_power,
        load_voltage,
        load_current,
        load_power,
        battery_v,
        hour,
    };
}

float SolarSim::solar_irradiance(float sim_hour) {
    if (sim_hour < 6.0f || sim_hour > 18.0f)
        return 0.0f;

    float phase = (sim_hour - 6.0f) / 12.0f;
    float base  = sinf(phase * PI);
    float value = base + gauss(0.0f, CLOUD_SIGMA);
    return (value > 0.0f) ? value : 0.0f;
}

float SolarSim::clamp(float value, float lo, float hi) {
    if (value < lo) return lo;
    if (value > hi) return hi;
    return value;
}

float SolarSim::gauss(float mean, float stddev) {
    // Box-Muller transform
    float u1 = (float)random(1, 10000) / 10000.0f;
    float u2 = (float)random(1, 10000) / 10000.0f;
    float z  = sqrtf(-2.0f * logf(u1)) * cosf(2.0f * PI * u2);
    return mean + stddev * z;
}
