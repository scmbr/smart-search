package main

import (
	"context"
	"net"
	"net/http"
	"strings"
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

func TestRunRejectsImplicitLiveModesBeforeDependencies(t *testing.T) {
	for _, args := range [][]string{{"-live-query"}, {"-parser-mode", "replay"}, {"-vector-corpus", "file", "-parser-mode", "live"}, {"-parser-mode", "bad"}} {
		err := run(context.Background(), args)
		if err == nil || (!strings.Contains(err.Error(), "requires") && !strings.Contains(err.Error(), "require") && !strings.Contains(err.Error(), "parser-mode")) {
			t.Fatal(args, err)
		}
	}
}
