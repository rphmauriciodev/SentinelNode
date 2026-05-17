package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// Testa se o codificador de heartbeat gera os bytes corretos de acordo com a especificação
func TestEncodeHeartbeat(t *testing.T) {
	deviceID := "camera-01"
	timestamp := time.Unix(1715974000, 0)
	status := "active"
	firmware := "1.0.0"

	bytesOut := EncodeHeartbeat(deviceID, timestamp, status, firmware)

	// Validações básicas de cabeçalho
	if len(bytesOut) < 12 {
		t.Fatalf("pacote retornado muito curto: %d bytes", len(bytesOut))
	}

	if bytesOut[0] != Magic1 || bytesOut[1] != Magic2 {
		t.Errorf("magic bytes incorretos: %x %x", bytesOut[0], bytesOut[1])
	}

	if bytesOut[2] != TypeHeartbeat {
		t.Errorf("tipo de pacote incorreto: %x", bytesOut[2])
	}

	// Valida se o timestamp foi gravado corretamente em Big Endian na posição certa (offset 3 a 11)
	var parsedTime int64
	reader := bytes.NewReader(bytesOut[3:11])
	if err := binary.Read(reader, binary.BigEndian, &parsedTime); err != nil {
		t.Fatalf("falha ao ler timestamp dos bytes: %v", err)
	}

	if parsedTime != timestamp.Unix() {
		t.Errorf("timestamp incorreto: esperado %d, obteve %d", timestamp.Unix(), parsedTime)
	}
}

func TestEncodeEvent(t *testing.T) {
	deviceID := "camera-01"
	now := time.Unix(1715974000, 0) // Usar hora fixa evita diferenças de milissegundos
	typeOfEvent := "motion_detected"
	mediaURL := "http://localhost:8080/record/12345.mp4"

	packet := EncodeEvent(deviceID, now, typeOfEvent, mediaURL)

	if len(packet) < 20 {
		t.Fatalf("Pacote muito curto. Tamanho: %d", len(packet))
	}
	if packet[0] != Magic1 || packet[1] != Magic2 {
		t.Fatalf("Magic Bytes inválidos: %x%x", packet[0], packet[1])
	}
	if packet[2] != TypeEvent {
		t.Fatalf("Tipo de pacote inválido. Esperado 0x02, obtido 0x%x", packet[2])
	}

	// 1. Validar o Timestamp (int64)
	timeBuf := make([]byte, 8)
	copy(timeBuf, packet[3:11])
	var unixTime int64
	buf := bytes.NewReader(timeBuf)
	if err := binary.Read(buf, binary.BigEndian, &unixTime); err != nil {
		t.Fatalf("Erro ao ler timestamp: %v", err)
	}

	if unixTime != now.Unix() {
		t.Fatalf("Timestamp inválido. Esperado %v, obtido %v", now.Unix(), unixTime)
	}

	reader := bytes.NewReader(packet[11:])
	devLen, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("Erro ao ler comprimento do DeviceID: %v", err)
	}

	devIDBytes := make([]byte, devLen)
	if _, err := reader.Read(devIDBytes); err != nil {
		t.Fatalf("Erro ao ler DeviceID: %v", err)
	}

	if string(devIDBytes) != deviceID {
		t.Fatalf("DeviceID inválido. Esperado %s, obtido %s", deviceID, string(devIDBytes))
	}

	evtByte, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("Erro ao ler tipo do evento: %v", err)
	}

	typeNum := uint8(2)
	if typeOfEvent == "motion_detected" {
		typeNum = 1
	}
	if evtByte != typeNum {
		t.Fatalf("Tipo de evento inválido. Esperado %d, obtido %d", typeNum, evtByte)
	}

	var urlLen uint16
	if err := binary.Read(reader, binary.BigEndian, &urlLen); err != nil {
		t.Fatalf("Erro ao ler comprimento da URL: %v", err)
	}

	if int(urlLen) != len(mediaURL) {
		t.Fatalf("Comprimento da URL inválido. Esperado %d, obtido %d", len(mediaURL), urlLen)
	}

	urlBuf := make([]byte, urlLen)
	if _, err := reader.Read(urlBuf); err != nil {
		t.Fatalf("Erro ao ler URL do vídeo: %v", err)
	}

	if string(urlBuf) != mediaURL {
		t.Fatalf("URL do vídeo inválida. Esperado %s, obtido %s", mediaURL, string(urlBuf))
	}
}
