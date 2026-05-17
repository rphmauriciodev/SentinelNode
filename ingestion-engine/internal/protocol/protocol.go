package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
)

const (
	Magic1        = 0x53
	Magic2        = 0x4E
	TypeHeartbeat = 0x01
	TypeEvent     = 0x02
)

func Decode(data []byte) (byte, interface{}, error) {
	if len(data) < 12 {
		return 0, nil, errors.New("packet too short")
	}

	if data[0] != Magic1 || data[1] != Magic2 {
		return 0, nil, errors.New("invalid magic number")
	}

	pType := data[2]
	reader := bytes.NewReader(data[3:])

	var unixTime int64
	if err := binary.Read(reader, binary.BigEndian, &unixTime); err != nil {
		return 0, nil, err
	}
	timestamp := time.Unix(unixTime, 0)

	devIdLen, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	devIdBytes := make([]byte, devIdLen)
	if _, err := reader.Read(devIdBytes); err != nil {
		return 0, nil, err
	}
	deviceID := string(devIdBytes)

	switch pType {
	case TypeHeartbeat:
		statusByte, err := reader.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		status := "inactive"
		if statusByte == 1 {
			status = "active"
		}

		fwBytes := make([]byte, 3)
		if _, err := reader.Read(fwBytes); err != nil {
			return 0, nil, err
		}
		firmware := fmt.Sprintf("%d.%d.%d", fwBytes[0], fwBytes[1], fwBytes[2])

		hb := models.Heartbeat{
			DeviceID:        deviceID,
			Timestamp:       timestamp,
			Status:          status,
			FirmwareVersion: firmware,
		}
		return pType, hb, nil

	case TypeEvent:
		evtByte, err := reader.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		eventType := "other"
		if evtByte == 1 {
			eventType = "motion_detected"
		}

		var urlLen uint16
		if err := binary.Read(reader, binary.BigEndian, &urlLen); err != nil {
			return 0, nil, err
		}

		urlBytes := make([]byte, urlLen)
		if _, err := reader.Read(urlBytes); err != nil {
			return 0, nil, err
		}
		videoURL := string(urlBytes)

		event := models.EventMessage{
			DeviceID:  deviceID,
			Timestamp: timestamp,
			EventType: eventType,
			VideoURL:  videoURL,
		}
		return pType, event, nil
	}

	return 0, nil, errors.New("unknown packet type")
}
