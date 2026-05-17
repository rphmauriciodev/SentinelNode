package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/rphmauriciodev/SentinelNode/ingestion-engine/internal/models"
)

func TestDecode_InvalidPacket(t *testing.T) {
	_, _, err := Decode([]byte{0x53, 0x4E})
	if err == nil {
		t.Error("esperava erro para pacote muito curto, mas obteve nil")
	}

	dummyData := make([]byte, 15)
	dummyData[0] = 0x00
	dummyData[1] = 0x00
	_, _, err = Decode(dummyData)
	if err == nil {
		t.Error("esperava erro para magic number inválido, mas obteve nil")
	}
}

func TestDecode_Heartbeat(t *testing.T) {
	buf := new(bytes.Buffer)
	buf.WriteByte(Magic1)
	buf.WriteByte(Magic2)
	buf.WriteByte(TypeHeartbeat)

	now := time.Now().Unix()
	binary.Write(buf, binary.BigEndian, now)

	devID := "camera-test"
	buf.WriteByte(byte(len(devID)))
	buf.WriteString(devID)

	buf.WriteByte(1)
	buf.WriteByte(1)
	buf.WriteByte(2)
	buf.WriteByte(3)

	pType, payload, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("erro inesperado ao decodificar: %v", err)
	}

	if pType != TypeHeartbeat {
		t.Errorf("esperava tipo %d, obteve %d", TypeHeartbeat, pType)
	}

	hb, ok := payload.(models.Heartbeat)
	if !ok {
		t.Fatal("esperava payload do tipo models.Heartbeat")
	}

	if hb.DeviceID != devID {
		t.Errorf("esperava device_id %s, obteve %s", devID, hb.DeviceID)
	}

	if hb.Status != "active" {
		t.Errorf("esperava status active, obteve %s", hb.Status)
	}

	if hb.FirmwareVersion != "1.2.3" {
		t.Errorf("esperava firmware 1.2.3, obteve %s", hb.FirmwareVersion)
	}
}
