package names

import (
	"context"
	"errors"
	d "github.com/scmbr/smart-search/internal/domain/search"
	"testing"
)

type sourceFunc func(context.Context) (Snapshot, error)

func (f sourceFunc) LoadNames(c context.Context) (Snapshot, error) { return f(c) }
func TestLoad(t *testing.T) {
	s := sourceFunc(func(context.Context) (Snapshot, error) {
		return Snapshot{"snapshot", []d.NameEntity{{Kind: "brand", Name: "Adobe", Scope: "/adobe/"}}}, nil
	})
	dictionary, err := Load(context.Background(), s, nil)
	if err != nil || dictionary.ResolveName("Adobe").Status != "resolved" {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Load(ctx, s, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	sentinel := errors.New("source failed")
	if _, err = Load(context.Background(), sourceFunc(func(context.Context) (Snapshot, error) { return Snapshot{}, sentinel }), nil); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err = Load(context.Background(), sourceFunc(func(context.Context) (Snapshot, error) { return Snapshot{}, nil }), nil); err == nil {
		t.Fatal("empty dictionary accepted")
	}
}
