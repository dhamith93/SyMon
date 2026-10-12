package api

import (
	"testing"
	"time"
)

func woken(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestTaskWaiters(t *testing.T) {
	var waiters taskWaiters
	// waking a host nobody waits for is fine
	waiters.wake("web1")

	web1 := waiters.wait("web1")
	again := waiters.wait("web1")
	db1 := waiters.wait("db1")
	if woken(web1) {
		t.Fatal("expected web1 to keep waiting")
	}
	waiters.wake("web1")
	if !woken(web1) || !woken(again) {
		t.Error("expected both of web1's calls to wake up")
	}
	if woken(db1) {
		t.Error("expected db1 to keep waiting")
	}
	// the next wait is for the next run
	if woken(waiters.wait("web1")) {
		t.Error("expected a new wait after a wake up to wait")
	}
}
