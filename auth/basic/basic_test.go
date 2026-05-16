package basic

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

func TestNewValidatesUsername(t *testing.T) {
	_, err := New("", "secret")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
	if !errors.Is(err, auth.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewAllowsEmptyPassword(t *testing.T) {
	src, err := New("alice", "")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("alice:"))
	if tok.Value != want {
		t.Errorf("Value = %q, want %q", tok.Value, want)
	}
	if tok.Type != TokenType {
		t.Errorf("Type = %q, want %q", tok.Type, TokenType)
	}
}

func TestTokenEncoding(t *testing.T) {
	src, err := New("alice", "s3cret:colon")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("alice:s3cret:colon"))
	if tok.Value != want {
		t.Errorf("Value = %q, want %q", tok.Value, want)
	}
}

func TestTokenHonoursContext(t *testing.T) {
	src, err := New("u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestTokenConcurrent(t *testing.T) {
	src, err := New("u", "p")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := src.Token(context.Background()); err != nil {
				t.Errorf("Token: %v", err)
			}
		}()
	}
	wg.Wait()
}
