#ifndef PROTOCOL_H
#define PROTOCOL_H

#include <stdint.h>
#include <string.h>

#ifdef __cplusplus
namespace Protocol {
#endif

#ifdef __cplusplus
const uint8_t Magic1 = 0x53;        // 'S'
const uint8_t Magic2 = 0x4E;        // 'N'
const uint8_t TypeHeartbeat = 0x01;
const uint8_t TypeEvent = 0x02;
#else
#define Magic1 ((uint8_t)0x53)
#define Magic2 ((uint8_t)0x4E)
#define TypeHeartbeat ((uint8_t)0x01)
#define TypeEvent ((uint8_t)0x02)
#endif

/**
 * Encodes a heartbeat packet into a binary buffer.
 * Packet structure:
 * [0]     : Magic1 (0x53 / 'S')
 * [1]     : Magic2 (0x4E / 'N')
 * [2]     : Type (0x01 / Heartbeat)
 * [3..10] : Timestamp (8 bytes uint64_t Big-Endian)
 * [11]    : Device ID Length (1 byte)
 * [12..]  : Device ID string (Variable, dev_len bytes)
 * [...]   : Status Byte (1 byte, 0x01 = active, 0x02 = inactive)
 * [...]   : Major Version (1 byte)
 * [...]   : Minor Version (1 byte)
 * [...]   : Patch Version (1 byte)
 * [...]   : Stream URL Length (2 bytes uint16_t Big-Endian)
 * [...]   : Stream URL string (Variable, url_len bytes)
 *
 * Returns the total number of bytes written to the buffer.
 */
static inline int encodeHeartbeat(uint8_t* buf, const char* device_id, uint64_t timestamp, uint8_t status, uint8_t major, uint8_t minor, uint8_t patch, const char* stream_url) {
    int offset = 0;
    buf[offset++] = Magic1;
    buf[offset++] = Magic2;
    buf[offset++] = TypeHeartbeat;

    // Encode timestamp (64-bit int) in Big-Endian order
    for (int i = 7; i >= 0; i--) {
        buf[offset++] = (timestamp >> (i * 8)) & 0xFF;
    }

    uint8_t dev_len = strlen(device_id);
    buf[offset++] = dev_len;
    memcpy(&buf[offset], device_id, dev_len);
    offset += dev_len;

    buf[offset++] = status;
    buf[offset++] = major;
    buf[offset++] = minor;
    buf[offset++] = patch;

    uint16_t url_len = strlen(stream_url);
    buf[offset++] = (url_len >> 8) & 0xFF;
    buf[offset++] = url_len & 0xFF;
    memcpy(&buf[offset], stream_url, url_len);
    offset += url_len;

    return offset;
}

/**
 * Encodes an event packet into a binary buffer.
 * Packet structure:
 * [0]     : Magic1 (0x53 / 'S')
 * [1]     : Magic2 (0x4E / 'N')
 * [2]     : Type (0x02 / Event)
 * [3..10] : Timestamp (8 bytes uint64_t Big-Endian)
 * [11]    : Device ID Length (1 byte)
 * [12..]  : Device ID string (Variable, dev_len bytes)
 * [...]   : Event Type Byte (1 byte, 0x01 = motion_detected, 0x02 = other)
 * [...]   : URL Length (2 bytes uint16_t Big-Endian)
 * [...]   : Video URL string (Variable, url_len bytes)
 * 
 * Returns the total number of bytes written to the buffer.
 */
static inline int encodeEvent(uint8_t* buf, const char* device_id, uint64_t timestamp, uint8_t event_type, const char* video_url) {
    int offset = 0;
    buf[offset++] = Magic1;
    buf[offset++] = Magic2;
    buf[offset++] = TypeEvent;

    // Encode timestamp (64-bit int) in Big-Endian order
    for (int i = 7; i >= 0; i--) {
        buf[offset++] = (timestamp >> (i * 8)) & 0xFF;
    }

    uint8_t dev_len = strlen(device_id);
    buf[offset++] = dev_len;
    memcpy(&buf[offset], device_id, dev_len);
    offset += dev_len;

    buf[offset++] = event_type;

    uint16_t url_len = strlen(video_url);
    buf[offset++] = (url_len >> 8) & 0xFF;
    buf[offset++] = url_len & 0xFF;
    memcpy(&buf[offset], video_url, url_len);
    offset += url_len;

    return offset;
}

#ifdef __cplusplus
} // namespace Protocol
#endif

#endif // PROTOCOL_H
