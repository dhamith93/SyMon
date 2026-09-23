package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestEnrollment(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	token, expires, err := st.CreateEnrollmentToken(ctx, "", 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expires) < 59*time.Minute || len(token) < 40 {
		t.Errorf("unexpected token %q expiring %v", token, expires)
	}

	secret, err := st.Enroll(ctx, token, "web1", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if host, err := st.HostForCredential(ctx, secret); err != nil || host != "web1" {
		t.Errorf("credential should belong to web1, got %q %v", host, err)
	}

	// single use
	if _, err := st.Enroll(ctx, token, "web2", "UTC"); !errors.Is(err, ErrBadToken) {
		t.Errorf("expected a used token to be rejected, got %v", err)
	}
	if _, err := st.Enroll(ctx, "not-a-token", "web2", "UTC"); !errors.Is(err, ErrBadToken) {
		t.Errorf("expected an unknown token to be rejected, got %v", err)
	}
	if _, err := st.HostForCredential(ctx, "not-a-secret"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected an unknown credential to be rejected, got %v", err)
	}
}

func TestReenrollKeepsTheHost(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	firstID, _ := st.hostID(ctx, "web1")

	tokens := make([]string, 2)
	for i := range tokens {
		tokens[i], _, _ = st.CreateEnrollmentToken(ctx, "", 1, time.Hour)
	}
	oldSecret, err := st.Enroll(ctx, tokens[0], "web1", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := st.Enroll(ctx, tokens[1], "web1", "Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}

	if id, _ := st.hostID(ctx, "web1"); id != firstID {
		t.Errorf("re-enrolling created a new host: %d, was %d", id, firstID)
	}
	if _, err := st.HostForCredential(ctx, oldSecret); !errors.Is(err, ErrNotFound) {
		t.Error("the old credential should stop working")
	}
	if host, _ := st.HostForCredential(ctx, newSecret); host != "web1" {
		t.Error("the new credential should work")
	}
}

func TestTokenLimits(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	bound, _, _ := st.CreateEnrollmentToken(ctx, "pi-garage", 1, time.Hour)
	if _, err := st.Enroll(ctx, bound, "web1", "UTC"); !errors.Is(err, ErrBadToken) {
		t.Errorf("a token for pi-garage must not enroll web1, got %v", err)
	}
	if _, err := st.Enroll(ctx, bound, "pi-garage", "UTC"); err != nil {
		t.Errorf("a token for pi-garage should enroll it, got %v", err)
	}

	expired, _, _ := st.CreateEnrollmentToken(ctx, "", 1, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if _, err := st.Enroll(ctx, expired, "web1", "UTC"); !errors.Is(err, ErrBadToken) {
		t.Errorf("expected an expired token to be rejected, got %v", err)
	}

	if _, _, err := st.CreateEnrollmentToken(ctx, "", 0, time.Hour); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected zero uses to be rejected, got %v", err)
	}
	if purged, err := st.PurgeEnrollmentTokens(ctx); err != nil || purged != 2 {
		t.Errorf("expected the used and the expired token purged, got %d %v", purged, err)
	}
}

// two hosts racing for the last use of a token: exactly one gets it
func TestTokenLastUseRace(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	token, _, _ := st.CreateEnrollmentToken(ctx, "", 1, time.Hour)

	var wg sync.WaitGroup
	results := make([]error, 5)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = st.Enroll(ctx, token, "host"+string(rune('a'+i)), "UTC")
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrBadToken) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("expected exactly one host to enroll, got %d", succeeded)
	}
}

func TestRemovingAHostRevokesItsCredential(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	token, _, _ := st.CreateEnrollmentToken(ctx, "", 1, time.Hour)
	secret, err := st.Enroll(ctx, token, "web1", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveHost(ctx, "web1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.HostForCredential(ctx, secret); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the credential to be gone, got %v", err)
	}
}
