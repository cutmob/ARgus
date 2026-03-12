package session

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// Manager handles all active inspection sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	profiles map[string]*EnvironmentProfile
	store    *memoryStore
}

func NewManager(memoryFilePath string) *Manager {
	mgr := &Manager{
		sessions: make(map[string]*Session),
		profiles: make(map[string]*EnvironmentProfile),
		store:    newMemoryStore(memoryFilePath),
	}
	if mgr.store != nil {
		if profiles, err := mgr.store.load(); err == nil {
			mgr.profiles = profiles
		} else {
			slog.Warn("failed to load environment memory", "error", err, "path", memoryFilePath)
		}
	}
	return mgr
}

// Create initializes a new inspection session.
func (m *Manager) Create(cfg SessionConfig) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &Session{
		ID:             cfg.SessionID,
		InspectionMode: cfg.InspectionMode,
		ActiveRuleset:  cfg.RulesetID,
		CameraID:       cfg.CameraID,
		State:          StateActive,
		Hazards:        make([]types.Hazard, 0),
		FrameBuffer:    NewFrameBuffer(cfg.BufferSize),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       cfg.Metadata,
	}

	m.sessions[s.ID] = s
	if cfg.CameraID != "" {
		profile := m.ensureEnvironmentProfileLocked(cfg.CameraID)
		profile.InspectionCount++
		profile.LastInspectionMode = cfg.InspectionMode
		profile.LastUpdated = time.Now()
		m.persistProfilesLocked()
	}
	slog.Info("session created", "id", s.ID, "mode", s.InspectionMode)
	return s
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Close terminates a session and finalizes its state.
func (m *Manager) Close(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	s.State = StateClosed
	s.ClosedAt = time.Now()
	slog.Info("session closed", "id", id, "hazards_found", len(s.Hazards))
	return s, true
}

// CloseAll terminates all active sessions.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		s.State = StateClosed
		s.ClosedAt = time.Now()
		slog.Info("session closed", "id", id)
	}
}

// List returns all sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// AddHazard records a detected hazard in the session.
func (m *Manager) AddHazard(sessionID string, hazard types.Hazard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		now := time.Now()
		hazard.DetectedAt = now
		if hazard.FirstSeenAt.IsZero() {
			hazard.FirstSeenAt = now
		}
		if hazard.LastSeenAt.IsZero() {
			hazard.LastSeenAt = now
		}
		if hazard.Occurrences == 0 {
			hazard.Occurrences = 1
		}

		if idx := findMatchingHazardIndex(s.Hazards, hazard); idx >= 0 {
			existing := &s.Hazards[idx]
			existing.LastSeenAt = now
			existing.DetectedAt = now
			existing.Occurrences++
			if existing.FirstSeenAt.IsZero() {
				existing.FirstSeenAt = now
			}
			existing.PersistenceSeconds = int(now.Sub(existing.FirstSeenAt).Seconds())
			if hazard.Confidence > existing.Confidence {
				existing.Confidence = hazard.Confidence
			}
			if severityRank(hazard.Severity) > severityRank(existing.Severity) {
				existing.Severity = hazard.Severity
				existing.RiskTrend = "rising"
			} else if severityRank(hazard.Severity) < severityRank(existing.Severity) {
				existing.RiskTrend = "falling"
			} else {
				existing.RiskTrend = "stable"
			}
			if hazard.RuleID != "" {
				existing.RuleID = hazard.RuleID
			}
			if hazard.CameraID != "" {
				existing.CameraID = hazard.CameraID
			}
			if hazard.Location != "" {
				existing.Location = hazard.Location
			}
			if hazard.BBox != nil {
				existing.BBox = hazard.BBox
			}
		} else {
			hazard.PersistenceSeconds = int(now.Sub(hazard.FirstSeenAt).Seconds())
			hazard.RiskTrend = "new"
			s.Hazards = append(s.Hazards, hazard)
		}
		cameraID := hazard.CameraID
		if cameraID == "" {
			cameraID = s.CameraID
		}
		m.recordEnvironmentHazardLocked(cameraID, s.InspectionMode, hazard, now)
		s.UpdatedAt = time.Now()
	}
}

func (m *Manager) UpdateMetadata(sessionID string, updates map[string]string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	for key, value := range updates {
		if value == "" {
			delete(s.Metadata, key)
			continue
		}
		s.Metadata[key] = value
	}
	s.UpdatedAt = time.Now()
	m.persistProfilesLocked()
	return true
}

func (m *Manager) GetEnvironmentProfile(cameraID string) (*EnvironmentProfile, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profile, ok := m.profiles[cameraID]
	if !ok {
		return nil, false
	}
	clone := *profile
	clone.FamiliarHazards = append([]EnvironmentHazardMemory(nil), profile.FamiliarHazards...)
	return &clone, true
}

func (m *Manager) ensureEnvironmentProfileLocked(cameraID string) *EnvironmentProfile {
	profile, ok := m.profiles[cameraID]
	if ok {
		return profile
	}
	profile = &EnvironmentProfile{
		CameraID: cameraID,
	}
	m.profiles[cameraID] = profile
	return profile
}

func (m *Manager) recordEnvironmentHazardLocked(cameraID, mode string, hazard types.Hazard, now time.Time) {
	if cameraID == "" {
		return
	}
	profile := m.ensureEnvironmentProfileLocked(cameraID)
	profile.ObservationCount++
	profile.LastInspectionMode = mode
	profile.LastUpdated = now
	description := strings.TrimSpace(hazard.Description)
	for i := range profile.FamiliarHazards {
		if strings.EqualFold(profile.FamiliarHazards[i].Description, description) {
			profile.FamiliarHazards[i].Count++
			profile.FamiliarHazards[i].LastSeenAt = now
			if severityRank(hazard.Severity) > severityRank(profile.FamiliarHazards[i].HighestSeverity) {
				profile.FamiliarHazards[i].HighestSeverity = hazard.Severity
			}
			return
		}
	}
	profile.FamiliarHazards = append(profile.FamiliarHazards, EnvironmentHazardMemory{
		Description:     description,
		Count:           1,
		HighestSeverity: hazard.Severity,
		LastSeenAt:      now,
	})
	m.persistProfilesLocked()
}

func (m *Manager) persistProfilesLocked() {
	if m.store == nil {
		return
	}
	profiles := make(map[string]*EnvironmentProfile, len(m.profiles))
	for key, profile := range m.profiles {
		if profile == nil {
			continue
		}
		clone := *profile
		clone.FamiliarHazards = append([]EnvironmentHazardMemory(nil), profile.FamiliarHazards...)
		profiles[key] = &clone
	}
	if err := m.store.save(profiles); err != nil {
		slog.Warn("failed to persist environment memory", "error", err)
	}
}

func findMatchingHazardIndex(hazards []types.Hazard, target types.Hazard) int {
	sig := hazardSignature(target)
	for i := range hazards {
		if hazardSignature(hazards[i]) == sig {
			return i
		}
	}
	return -1
}

func hazardSignature(h types.Hazard) string {
	key := strings.ToLower(strings.TrimSpace(h.Description))
	return strings.Join([]string{h.RuleID, h.CameraID, key}, "|")
}

func severityRank(s types.Severity) int {
	switch s {
	case types.SeverityLow:
		return 1
	case types.SeverityMedium:
		return 2
	case types.SeverityHigh:
		return 3
	case types.SeverityCritical:
		return 4
	default:
		return 0
	}
}

// HTTP Handlers

func (m *Manager) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions := m.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

func (m *Manager) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Extract session ID from path: /api/v1/sessions/{id}
	id := r.URL.Path[len("/api/v1/sessions/"):]
	s, ok := m.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}
