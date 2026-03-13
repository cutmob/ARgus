package temporal

import (
	"errors"
	"sync"
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// memoryStore is a simple in-memory Store implementation.
// It is intended for early wiring, tests, and development. A production
// deployment should provide a persistent implementation.
type memoryStore struct {
	mu        sync.RWMutex
	hazards   map[string][]types.Hazard      // sessionID -> hazards
	incidents map[string]Incident           // incidentID -> incident
	bySession map[string]map[string]bool    // sessionID -> incidentID set
	evidence  map[string]EvidencePack       // incidentID -> evidence
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		hazards:   make(map[string][]types.Hazard),
		incidents: make(map[string]Incident),
		bySession: make(map[string]map[string]bool),
		evidence:  make(map[string]EvidencePack),
	}
}

func (s *memoryStore) SaveHazard(sessionID string, hazard types.Hazard) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hazards[sessionID] = append(s.hazards[sessionID], hazard)
	return nil
}

func (s *memoryStore) UpsertIncident(incident Incident) (Incident, error) {
	if s == nil {
		return incident, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents[incident.IncidentID] = incident
	if _, ok := s.bySession[incident.SessionID]; !ok {
		s.bySession[incident.SessionID] = make(map[string]bool)
	}
	s.bySession[incident.SessionID][incident.IncidentID] = true
	return incident, nil
}

func (s *memoryStore) GetIncident(id string) (Incident, error) {
	if s == nil {
		return Incident{}, errors.New("store not initialized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	inc, ok := s.incidents[id]
	if !ok {
		return Incident{}, errors.New("incident not found")
	}
	return inc, nil
}

func (s *memoryStore) GetActiveIncidents(sessionID string) ([]Incident, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.bySession[sessionID]
	if !ok {
		return nil, nil
	}
	result := make([]Incident, 0, len(ids))
	for id := range ids {
		if inc, ok := s.incidents[id]; ok && inc.EndAt == nil {
			result = append(result, inc)
		}
	}
	return result, nil
}

func (s *memoryStore) GetIncidentsInRange(sessionID string, from, to time.Time) ([]Incident, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.bySession[sessionID]
	if !ok {
		return nil, nil
	}
	var result []Incident
	for id := range ids {
		inc, ok := s.incidents[id]
		if !ok {
			continue
		}
		if inc.StartAt.Before(to) && (inc.EndAt == nil || inc.EndAt.After(from)) {
			result = append(result, inc)
		}
	}
	return result, nil
}

func (s *memoryStore) SaveEvidence(pack EvidencePack) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[pack.IncidentID] = pack
	return nil
}

func (s *memoryStore) GetEvidence(incidentID string) (EvidencePack, error) {
	if s == nil {
		return EvidencePack{}, errors.New("store not initialized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	pack, ok := s.evidence[incidentID]
	if !ok {
		// Evidence is optional; return empty pack without error.
		return EvidencePack{}, nil
	}
	return pack, nil
}

