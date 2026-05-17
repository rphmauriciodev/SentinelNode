package protocol

import (
	"bytes"
	"encoding/binary"
	"time"
)

const (
	Magic1        = 0x53
	Magic2        = 0x4E
	TypeHeartbeat = 0x01
	TypeEvent     = 0x02
)

func EncodeHeartbeat(deviceID string, timestamp time.Time, status string, firmware string) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte(Magic1)
	buf.WriteByte(Magic2)

	buf.WriteByte(TypeHeartbeat)

	binary.Write(buf, binary.BigEndian, timestamp.Unix())

	buf.WriteByte(byte(len(deviceID)))
	buf.WriteString(deviceID)

	statusByte := byte(2)
	if status == "active" {
		statusByte = 1
	}
	buf.WriteByte(statusByte)

	var major, minor, patch byte = 1, 0, 0
	buf.WriteByte(major)
	buf.WriteByte(minor)
	buf.WriteByte(patch)

	return buf.Bytes()
}

func EncodeEvent(deviceID string, timestamp time.Time, eventType string, videoURL string) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte(Magic1)
	buf.WriteByte(Magic2)

	buf.WriteByte(TypeEvent)

	binary.Write(buf, binary.BigEndian, timestamp.Unix())

	buf.WriteByte(byte(len(deviceID)))
	buf.WriteString(deviceID)

	evtByte := byte(2)
	if eventType == "motion_detected" {
		evtByte = 1
	}
	buf.WriteByte(evtByte)

	binary.Write(buf, binary.BigEndian, uint16(len(videoURL)))
	buf.WriteString(videoURL)

	return buf.Bytes()
}
