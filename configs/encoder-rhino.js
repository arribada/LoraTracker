function Encode(fPort, obj) {
    var bytes = [];
    //settings
    if (fPort === 3) {
        bytes[0] = (obj.system_status_interval) & 0xFF;
        bytes[1] = (obj.system_status_interval) >> 8 & 0xFF;

        bytes[2] |= obj.system_functions.accelerometer_enabled ? 1 << 3 : 0;
        bytes[2] |= obj.system_functions.light_enabled ? 1 << 4 : 0;
        bytes[2] |= obj.system_functions.temperature_enabled ? 1 << 5 : 0;
        bytes[2] |= obj.system_functions.humidity_enabled ? 1 << 6 : 0;
        bytes[2] |= obj.system_functions.charging_enabled ? 1 << 7 : 0;

        bytes[3] |= (obj.lorawan_datarate_adr.datarate) & 0x0F;
        bytes[3] |= obj.lorawan_datarate_adr.confirmed_uplink ? 1 << 6 : 0;
        bytes[3] |= obj.lorawan_datarate_adr.adr ? 1 << 7 : 0;

        bytes[4] = (obj.gps_periodic_interval) & 0xFF;
        bytes[5] = (obj.gps_periodic_interval) >> 8 & 0xFF;

        bytes[6] = (obj.gps_triggered_interval) & 0xFF;
        bytes[7] = (obj.gps_triggered_interval) >> 8 & 0xFF;

        bytes[8] = (obj.gps_triggered_threshold) & 0xFF;

        bytes[9] = (obj.gps_triggered_duration) & 0xFF;

        bytes[10] = (obj.gps_cold_fix_timeout) & 0xFF;
        bytes[11] = (obj.gps_cold_fix_timeout) >> 8 & 0xFF;

        bytes[12] = (obj.gps_hot_fix_timeout) & 0xFF;
        bytes[13] = (obj.gps_hot_fix_timeout) >> 8 & 0xFF;

        bytes[14] = (obj.gps_min_fix_time) & 0xFF;

        bytes[15] = (obj.gps_min_ehpe) & 0xFF;

        bytes[16] = (obj.gps_hot_fix_retry) & 0xFF;

        bytes[17] = (obj.gps_cold_fix_retry) & 0xFF;

        bytes[18] = (obj.gps_fail_retry) & 0xFF;

        bytes[19] = obj.gps_settings.d3_fix ? 1 << 0 : 0;
        bytes[19] |= obj.gps_settings.fail_backoff ? 1 << 1 : 0;
        bytes[19] |= obj.gps_settings.hot_fix ? 1 << 2 : 0;
        bytes[19] |= obj.gps_settings.fully_resolved ? 1 << 3 : 0;
        bytes[20] = (obj.system_voltage_interval) & 0xFF;
        bytes[21] = ((obj.gps_charge_min - 2500) / 10) & 0xFF;
        bytes[22] = ((obj.system_charge_min - 2500) / 10) & 0xFF;
        bytes[23] = ((obj.system_charge_max - 2500) / 10) & 0xFF;
        bytes[24] = (obj.system_input_charge_min) & 0xFF;
        bytes[25] = (obj.system_input_charge_min) >> 8 & 0xFF;
    }
    else if (fPort === 30) {
        bytes[0] = (obj.freq_start) & 0xFF;
        bytes[1] = (obj.freq_start) >> 8 & 0xFF;
        bytes[2] = (obj.freq_start) >> 16 & 0xFF;
        bytes[3] = (obj.freq_start) >> 24 & 0xFF;

        bytes[4] = (obj.freq_stop) & 0xFF;
        bytes[5] = (obj.freq_stop) >> 8 & 0xFF;
        bytes[6] = (obj.freq_stop) >> 16 & 0xFF;
        bytes[7] = (obj.freq_stop) >> 24 & 0xFF;

        bytes[8] = (obj.samples) & 0xFF;
        bytes[9] = (obj.samples) >> 8 & 0xFF;
        bytes[10] = (obj.samples) >> 16 & 0xFF;
        bytes[11] = (obj.samples) >> 24 & 0xFF;

        bytes[12] = (obj.power) & 0xFF;
        bytes[13] = (obj.power) >> 8 & 0xFF;

        bytes[14] = (obj.time) & 0xFF;
        bytes[15] = (obj.time) >> 8 & 0xFF;

        bytes[16] = (obj.type) & 0xFF;
        bytes[17] = (obj.type) >> 8 & 0xFF;
    }
    //command
    else if (fPort === 99) {
        if (obj.command.reset) {
            bytes[0] = 0xab;
        }
        else if (obj.command.lora_rejoin) {
            bytes[0] = 0xde;
        }
        else if (obj.command.send_settings) {
            bytes[0] = 0xaa;
        }
    }
    return bytes;
}