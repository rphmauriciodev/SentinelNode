package stream

import (
	"sync"
)

type StreamManager struct {
	mu      sync.RWMutex
	streams map[string]*CameraStream
}

func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[string]*CameraStream),
	}
}

func (sm *StreamManager) Register(deviceID, sourceURL string) *CameraStream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if existing, found := sm.streams[deviceID]; found {
		if existing.SourceURL() != sourceURL && sourceURL != "" {
			existing.UpdateSourceURL(sourceURL)
		}
		return existing
	}

	cs := NewCameraStream(deviceID, sourceURL)
	sm.streams[deviceID] = cs
	return cs
}

func (sm *StreamManager) Get(deviceID string) (*CameraStream, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	cs, found := sm.streams[deviceID]
	return cs, found
}

func (sm *StreamManager) GetAll() []*CameraStream {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	list := make([]*CameraStream, 0, len(sm.streams))
	for _, cs := range sm.streams {
		list = append(list, cs)
	}
	return list
}

func (sm *StreamManager) Remove(deviceID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if cs, found := sm.streams[deviceID]; found {
		cs.Stop()
		delete(sm.streams, deviceID)
	}
}

func (sm *StreamManager) Close() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, cs := range sm.streams {
		cs.Stop()
	}
	sm.streams = make(map[string]*CameraStream)
}
