package temporal

import (
	"time"

	"github.com/cutmob/argus/pkg/types"
)

// Store abstracts persistence for temporal incidents and evidence.
// A production implementation can use Postgres or a time-series DB;
// tests can use an in-memory implementation.
type Store interface {
	// Hazard stream access (optional in v1, but kept for flexibility).
	SaveHazard(sessionID string, hazard types.Hazard) error

	// Incident persistence.
	UpsertIncident(incident Incident) (Incident, error)
	GetIncident(id string) (Incident, error)
	GetActiveIncidents(sessionID string) ([]Incident, error)
	GetIncidentsInRange(sessionID string, from, to time.Time) ([]Incident, error)

	// Evidence persistence.
	SaveEvidence(pack EvidencePack) error
	GetEvidence(incidentID string) (EvidencePack, error)
}

