package main

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadPirMappings_Default(t *testing.T) {
	os.Unsetenv("PIR_CAMERA_MAPPING")
	mappings := loadPirMappings()

	expected := map[string][]string{
		"esp32-pir-01": {"esp32-cam-01"},
	}

	if !reflect.DeepEqual(mappings, expected) {
		t.Fatalf("Esperava mapeamento padrão %+v, obteve %+v", expected, mappings)
	}
}

func TestLoadPirMappings_OneToMultipleCameras(t *testing.T) {
	os.Setenv("PIR_CAMERA_MAPPING", "esp32-pir-01:esp32-cam-01,esp32-cam-02;esp32-pir-sala:cam-frontal,cam-lateral,cam-fundo")
	defer os.Unsetenv("PIR_CAMERA_MAPPING")

	mappings := loadPirMappings()

	expected := map[string][]string{
		"esp32-pir-01":   {"esp32-cam-01", "esp32-cam-02"},
		"esp32-pir-sala": {"cam-frontal", "cam-lateral", "cam-fundo"},
	}

	if !reflect.DeepEqual(mappings, expected) {
		t.Fatalf("Esperava %+v, obteve %+v", expected, mappings)
	}
}
