package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type memoryStore struct {
	mu       sync.Mutex
	filePath string
}

func newMemoryStore(filePath string) *memoryStore {
	return &memoryStore{filePath: filePath}
}

func (ms *memoryStore) load() (map[string]*EnvironmentProfile, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.filePath == "" {
		return map[string]*EnvironmentProfile{}, nil
	}
	data, err := os.ReadFile(ms.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*EnvironmentProfile{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]*EnvironmentProfile{}, nil
	}
	var payload struct {
		Profiles map[string]*EnvironmentProfile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Profiles == nil {
		payload.Profiles = map[string]*EnvironmentProfile{}
	}
	for key, profile := range payload.Profiles {
		if profile == nil {
			delete(payload.Profiles, key)
			continue
		}
		if profile.CameraID == "" {
			profile.CameraID = key
		}
	}
	return payload.Profiles, nil
}

func (ms *memoryStore) save(profiles map[string]*EnvironmentProfile) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.filePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ms.filePath), 0o755); err != nil {
		return err
	}
	payload := struct {
		SavedAt  time.Time                       `json:"saved_at"`
		Profiles map[string]*EnvironmentProfile `json:"profiles"`
	}{
		SavedAt:  time.Now().UTC(),
		Profiles: profiles,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ms.filePath, data, 0o600)
}
