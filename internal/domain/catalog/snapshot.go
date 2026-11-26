package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	ErrBusy           = errors.New("another catalog import is running")
	ErrInvalidCatalog = errors.New("catalog failed validation")
	ErrCheckpoint     = errors.New("catalog checkpoint mismatch")
)

type Identity struct {
	SourceSHA256  string
	SchemaVersion string
}

func (i Identity) ID() string {
	h := sha256.Sum256([]byte(i.SchemaVersion + "\x00" + i.SourceSHA256))
	return hex.EncodeToString(h[:])
}

type Status string

const (
	Staging Status = "staging"
	Ready   Status = "ready"
	Invalid Status = "invalid"
)

type Issue struct{ Code, EntityID, Message string }

type Validation struct {
	Complete                                       bool
	CatalogDate                                    string
	Categories, Offers, Accepted, Rejected, Issues int
	Diagnostics                                    []Issue
}

func (v Validation) Eligible() bool {
	return v.Complete && v.Categories > 0 && v.Accepted > 0 &&
		v.Offers == v.Accepted && v.Rejected == 0 && v.Issues == 0
}

type Snapshot struct {
	ID         string
	Status     Status
	Checkpoint int64
	Validation Validation
}

// Repository serializes writers; an ImportSession owns the writer lock until Close.
type Repository interface {
	Begin(context.Context, Identity) (ImportSession, error)
}

type ImportSession interface {
	Snapshot() Snapshot
	// Save atomically persists consecutive records and advances the checkpoint.
	Save(ctx context.Context, first int64, records []Record) error
	// Finish atomically validates saved counts and publishes eligible snapshots.
	Finish(context.Context, Validation) error
	Close() error
}
