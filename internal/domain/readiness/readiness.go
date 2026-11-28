// Package readiness owns consistency of the mandatory search snapshots.
package readiness

import "errors"

var ErrNotReady = errors.New("required search resources not ready")

type State struct {
	CatalogID string
	Ready     bool
}

func Check(catalog, index State) error {
	if !catalog.Ready || !index.Ready || catalog.CatalogID == "" || catalog.CatalogID != index.CatalogID {
		return ErrNotReady
	}
	return nil
}
