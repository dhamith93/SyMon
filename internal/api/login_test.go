package api

import (
	"testing"
	"time"
)

func TestLoginFailures(t *testing.T) {
	now := time.Unix(1700000000, 0)
	failures := &loginFailures{now: func() time.Time { return now }}

	for i := 0; i < loginLimit-1; i++ {
		failures.fail("alice")
	}
	if failures.blocked("alice") {
		t.Fatal("expected alice to have one more try")
	}
	failures.fail("alice")
	if !failures.blocked("alice") || failures.blocked("bob") {
		t.Fatal("expected only alice to be locked out")
	}

	// the lock ends with the window
	now = now.Add(loginWindow)
	if failures.blocked("alice") {
		t.Error("expected alice's lock to end with the window")
	}
	failures.fail("alice")
	if failures.blocked("alice") {
		t.Error("expected a new window to start counting again")
	}

	// a good login clears the count
	failures.clear("alice")
	for i := 0; i < loginLimit-1; i++ {
		failures.fail("alice")
	}
	if failures.blocked("alice") {
		t.Error("expected the count to start again after a good login")
	}
}
