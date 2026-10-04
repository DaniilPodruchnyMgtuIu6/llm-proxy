package logging

import (
	"context"
	"testing"
)

func TestParseLevel(t *testing.T) {
	if ParseLevel("DEBUG") != LevelDebug {
		t.Fatal("debug")
	}
	if ParseLevel("info") != LevelInfo {
		t.Fatal("info")
	}
	if ParseLevel("error") != LevelError {
		t.Fatal("error")
	}
}

func TestEnabledByLevel(t *testing.T) {
	Configure("info")
	if !Enabled(LevelError) || !Enabled(LevelInfo) || Enabled(LevelDebug) {
		t.Fatalf("info floor wrong: err=%v info=%v dbg=%v", Enabled(LevelError), Enabled(LevelInfo), Enabled(LevelDebug))
	}
	Configure("debug")
	if !Enabled(LevelDebug) {
		t.Fatal("debug should enable debug")
	}
	Configure("error")
	if Enabled(LevelInfo) {
		t.Fatal("error floor should hide info")
	}
}

func TestResolveRequestID(t *testing.T) {
	got := ResolveRequestID("client-abc_123")
	if got != "client-abc_123" {
		t.Fatalf("got %q", got)
	}
	if id := ResolveRequestID("bad id!"); id == "bad id!" {
		t.Fatal("invalid header accepted")
	}
	gen := ResolveRequestID("")
	if gen == "" || len(gen) < 8 {
		t.Fatalf("expected generated id, got %q", gen)
	}
}

func TestContextRequestID(t *testing.T) {
	ctx := WithRequestID(context.Background(), "r1")
	if RequestID(ctx) != "r1" {
		t.Fatal(RequestID(ctx))
	}
}
