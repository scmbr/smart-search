package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerStopsAfterCancellation(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, l, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	r, err := client.Get("http://" + l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("server did not stop")
	}
}
func TestRunRejectsPublicListenerBeforeDependencies(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8080", "[::]:8080", "example.com:8080", "invalid"} {
		if run(context.Background(), []string{"-listen", addr}) == nil {
			t.Fatal("accepted", addr)
		}
	}
}
