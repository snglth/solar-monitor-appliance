#pragma once

#include <Arduino.h>

struct SolarReadings {
    float pv_voltage;
    float pv_current;
    float pv_power;
    float load_voltage;
    float load_current;
    float load_power;
    float battery_voltage;
    float sim_hour;
};

class SolarSim {
public:
    SolarSim();
    SolarReadings update(float dt_seconds);

private:
    float battery_v;
    float load_current;
    float sim_time;  // seconds into simulated day

    static constexpr float IMP         = 1.71f;   // panel max-power current (A)
    static constexpr float BATTERY_REST = 12.8f;
    static constexpr float BATTERY_MIN  = 11.5f;
    static constexpr float BATTERY_MAX  = 14.4f;
    static constexpr float CAPACITY_AH  = 18.0f;
    static constexpr float LOAD_BASE_A  = 1.0f;
    static constexpr float INA_NOISE    = 0.010f;  // INA219 noise sigma (A)
    static constexpr float CLOUD_SIGMA  = 0.03f;

    float solar_irradiance(float sim_hour);
    static float clamp(float value, float lo, float hi);
    float gauss(float mean, float stddev);
};
