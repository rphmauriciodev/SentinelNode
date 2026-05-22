#include <iostream>
#include <cassert>
#include <iomanip>
#include "protocol.h"

// Helper function to print hex bytes
void printHex(const uint8_t* buf, int size) {
    for (int i = 0; i < size; i++) {
        std::cout << "0x" << std::hex << std::setw(2) << std::setfill('0') << (int)buf[i] << " ";
    }
    std::cout << std::dec << std::endl;
}

void test_encode_heartbeat() {
    std::cout << "🧪 Rodando: test_encode_heartbeat()..." << std::endl;

    uint8_t buf[256] = {0};
    const char* device_id = "esp32-cam-01";
    uint64_t timestamp = 1718928374ULL; // 0x000000006674C7F6
    uint8_t status = 0x01; // active
    uint8_t major = 1;
    uint8_t minor = 0;
    uint8_t patch = 0;
    const char* stream_url = "http://192.168.1.100/stream";

    int size = Protocol::encodeHeartbeat(buf, device_id, timestamp, status, major, minor, patch, stream_url);

    std::cout << "   ↳ Gerado: ";
    printHex(buf, size);

    // Expected size: Magic(2) + Type(1) + Timestamp(8) + DevLen(1) + DeviceID(12) + Status(1) + Version(3) + UrlLen(2) + URL(27) = 55
    int expected_size = 3 + 8 + 1 + 12 + 1 + 3 + 2 + 27;
    assert(size == expected_size);

    // Assert Magic numbers
    assert(buf[0] == 0x53); // 'S'
    assert(buf[1] == 0x4E); // 'N'
    // Assert Type
    assert(buf[2] == 0x01); // Heartbeat

    // Assert Timestamp (Big-Endian)
    assert(buf[3] == 0x00);
    assert(buf[4] == 0x00);
    assert(buf[5] == 0x00);
    assert(buf[6] == 0x00);
    assert(buf[7] == 0x66);
    assert(buf[8] == 0x74);
    assert(buf[9] == 0xC3);
    assert(buf[10] == 0xF6);

    // Assert Device ID length and value
    assert(buf[11] == 12);
    assert(memcmp(&buf[12], device_id, 12) == 0);

    // Assert Status
    assert(buf[24] == 0x01);

    // Assert Version
    assert(buf[25] == 1);
    assert(buf[26] == 0);
    assert(buf[27] == 0);

    // Assert Stream URL Length (2 bytes Big-Endian: 27 = 0x001B)
    assert(buf[28] == 0x00);
    assert(buf[29] == 27);

    // Assert Stream URL Content
    assert(memcmp(&buf[30], stream_url, 26) == 0);

    std::cout << "   [PASS] Heartbeat binário codificado corretamente (" << size << " bytes):" << std::endl;
    std::cout << "   ↳ ";
    printHex(buf, size);
}

void test_encode_event() {
    std::cout << "🧪 Rodando: test_encode_event()..." << std::endl;

    uint8_t buf[256] = {0};
    const char* device_id = "esp32-cam-01";
    uint64_t timestamp = 1718928374ULL; // 0x000000006674C7F6
    uint8_t event_type = 0x01; // motion_detected
    const char* video_url = "http://192.168.1.100/stream"; // Length: 27 (0x001B)

    int size = Protocol::encodeEvent(buf, device_id, timestamp, event_type, video_url);

    std::cout << "   ↳ Gerado: ";
    printHex(buf, size);

    // Expected size: Magic(2) + Type(1) + Timestamp(8) + DevLen(1) + DeviceID(12) + EvtType(1) + UrlLen(2) + URL(27) = 44
    int expected_size = 3 + 8 + 1 + 12 + 1 + 2 + 27;
    assert(size == expected_size);

    // Assert Magic numbers
    assert(buf[0] == 0x53); // 'S'
    assert(buf[1] == 0x4E); // 'N'
    // Assert Type
    assert(buf[2] == 0x02); // Event

    // Assert Timestamp (Big-Endian)
    assert(buf[3] == 0x00);
    assert(buf[4] == 0x00);
    assert(buf[5] == 0x00);
    assert(buf[6] == 0x00);
    assert(buf[7] == 0x66);
    assert(buf[8] == 0x74);
    assert(buf[9] == 0xC3);
    assert(buf[10] == 0xF6);

    // Assert Device ID length and value
    assert(buf[11] == 12);
    assert(memcmp(&buf[12], device_id, 12) == 0);

    // Assert Event Type
    assert(buf[24] == 0x01);

    // Assert URL Length (2 bytes Big-Endian: 27 = 0x001B)
    assert(buf[25] == 0x00);
    assert(buf[26] == 27);

    // Assert URL Content
    assert(memcmp(&buf[27], video_url, 27) == 0);

    std::cout << "   [PASS] Event binário codificado corretamente (" << size << " bytes):" << std::endl;
    std::cout << "   ↳ ";
    printHex(buf, size);
}

int main() {
    std::cout << "🟢 INICIANDO TESTES DO PROTOCOLO C++ SENTINELNODE" << std::endl;
    std::cout << "=================================================" << std::endl;
    
    test_encode_heartbeat();
    test_encode_event();
    
    std::cout << "=================================================" << std::endl;
    std::cout << "🎉 TODOS OS TESTES UNITÁRIOS C++ PASSARAM COM SUCESSO!" << std::endl;
    return 0;
}
