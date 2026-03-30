package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestGetSetJSON(t *testing.T) {
	mr := miniredis.RunT(t)
	client := NewFromOptions(mr.Addr(), "", 0)

	ctx := context.Background()
	type payload struct {
		Value string `json:"value"`
	}

	if err := client.SetJSON(ctx, "demo", payload{Value: "ok"}, time.Minute); err != nil {
		t.Fatalf("set json: %v", err)
	}

	var result payload
	hit, err := client.GetJSON(ctx, "demo", &result)
	if err != nil {
		t.Fatalf("get json: %v", err)
	}
	if !hit {
		t.Fatalf("expected cache hit")
	}
	if result.Value != "ok" {
		t.Fatalf("unexpected cached value: %s", result.Value)
	}
}

func TestIncrementWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := NewFromOptions(mr.Addr(), "", 0)

	ctx := context.Background()
	count, err := client.IncrementWindow(ctx, "rate:test", time.Minute)
	if err != nil {
		t.Fatalf("increment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected first count to be 1, got %d", count)
	}

	count, err = client.IncrementWindow(ctx, "rate:test", time.Minute)
	if err != nil {
		t.Fatalf("increment second: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected second count to be 2, got %d", count)
	}
}

func TestDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	client := NewFromOptions(mr.Addr(), "", 0)

	ctx := context.Background()
	if err := client.SetJSON(ctx, "demo-delete", map[string]string{"ok": "1"}, time.Minute); err != nil {
		t.Fatalf("set json: %v", err)
	}
	if err := client.Delete(ctx, "demo-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var result map[string]string
	hit, err := client.GetJSON(ctx, "demo-delete", &result)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if hit {
		t.Fatalf("expected cache miss after delete")
	}
}

func TestSetStringAndExists(t *testing.T) {
	mr := miniredis.RunT(t)
	client := NewFromOptions(mr.Addr(), "", 0)

	ctx := context.Background()
	if err := client.SetString(ctx, "token:blacklist:test", "1", time.Minute); err != nil {
		t.Fatalf("set string: %v", err)
	}

	exists, err := client.Exists(ctx, "token:blacklist:test")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected key to exist")
	}
}
