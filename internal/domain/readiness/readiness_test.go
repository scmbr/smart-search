package readiness

import (
	"errors"
	"testing"
)

func TestMatchingReadySnapshots(t *testing.T) {
	if err := Check(State{CatalogID: "a", Ready: true}, State{CatalogID: "a", Ready: true}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ catalog, index State }{
		{State{}, State{}},
		{State{CatalogID: "a", Ready: true}, State{CatalogID: "b", Ready: true}},
		{State{CatalogID: "a", Ready: true}, State{CatalogID: "a"}},
		{State{CatalogID: "a"}, State{CatalogID: "a", Ready: true}},
		{State{Ready: true}, State{Ready: true}},
	} {
		if !errors.Is(Check(tc.catalog, tc.index), ErrNotReady) {
			t.Fatal("accepted inconsistent state", tc)
		}
	}
}
