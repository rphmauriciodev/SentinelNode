#include <Arduino.h>
#include <WiFi.h>

extern "C" void setup_c();
extern "C" void loop_c();

extern "C" void conectar_wifi_cpp(const char* ssid, const char* password) {
    WiFi.mode(WIFI_STA);
    WiFi.begin(ssid, password);
}

extern "C" bool wifi_conectado_cpp() {
    return WiFi.status() == WL_CONNECTED;
}

extern "C" int obter_status_wifi_cpp() {
    return (int)WiFi.status();
}

extern "C" void obter_ip_cpp(char* ip_str, size_t max_len) {
    if (WiFi.status() == WL_CONNECTED) {
        String ip = WiFi.localIP().toString();
        strncpy(ip_str, ip.c_str(), max_len - 1);
        ip_str[max_len - 1] = '\0';
    } else {
        strncpy(ip_str, "0.0.0.0", max_len - 1);
    }
}

void setup() {
    Serial.begin(115200);
    Serial.flush();
    delay(300);
    setup_c();
}

void loop() {
    loop_c();
}
