package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type RecordingMetadata struct {
	ID              string    `json:"id"`
	DeviceID        string    `json:"device_id"`
	FilePath        string    `json:"file_path"`
	FileName        string    `json:"file_name"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds float64   `json:"duration_seconds"`
	SizeBytes       int64     `json:"size_bytes"`
	Status          string    `json:"status"`
	ErrorReason     string    `json:"error_reason,omitempty"`
}

type MetadataStore struct {
	mu            sync.RWMutex
	recordings    map[string]*RecordingMetadata
	recordingsDir string
}

func NewMetadataStore(recordingsDir string) *MetadataStore {
	store := &MetadataStore{
		recordings:    make(map[string]*RecordingMetadata),
		recordingsDir: recordingsDir,
	}
	store.loadExistingFiles()
	return store
}

func (s *MetadataStore) Save(meta *RecordingMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordings[meta.ID] = meta
}

func (s *MetadataStore) Get(id string) (*RecordingMetadata, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meta, found := s.recordings[id]
	return meta, found
}

func (s *MetadataStore) GetAll() []*RecordingMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*RecordingMetadata, 0, len(s.recordings))
	for _, meta := range s.recordings {
		list = append(list, meta)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].StartedAt.After(list[j].StartedAt)
	})
	return list
}

func (s *MetadataStore) loadExistingFiles() {
	files, err := os.ReadDir(s.recordingsDir)
	if err != nil {
		return
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".mp4" {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(s.recordingsDir, f.Name())
		id := f.Name()
		s.recordings[id] = &RecordingMetadata{
			ID:              id,
			DeviceID:        "unknown",
			FilePath:        fullPath,
			FileName:        f.Name(),
			StartedAt:       info.ModTime(),
			EndedAt:         info.ModTime(),
			DurationSeconds: 0,
			SizeBytes:       info.Size(),
			Status:          "completed",
		}
	}
}

func (s *MetadataStore) PersistManifest() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	manifestPath := filepath.Join(s.recordingsDir, "manifest.json")
	data, err := json.MarshalIndent(s.recordings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0644)
}
