package models

import "time"

type Heartbeat struct {
	DeviceID        string    `json:"device_id"`
	Timestamp       time.Time `json:"timestamp"`
	Status          string    `json:"status"`
	FirmwareVersion string    `json:"firmware_version"`
}

type EventMessage struct {
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	VideoURL  string    `json:"video_url"`
}

type RecordRequest struct {
	DeviceID        string `json:"device_id"`
	VideoURL        string `json:"video_url"`
	DurationSeconds int    `json:"duration_seconds"`
}
