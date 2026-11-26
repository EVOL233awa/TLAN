//go:build windows

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestSingleInstanceSignaling(t *testing.T) {
	suffix := fmt.Sprintf("Test-%d", time.Now().UnixNano())
	muxName := `Local\TLAN-Desktop-` + suffix
	evName := `Local\TLAN-Desktop-Activate-` + suffix

	mux, ev, already, err := acquireSingleInstanceNamed(muxName, evName)
	if err != nil {
		t.Fatalf("acquire primary failed: %v", err)
	}
	if already {
		t.Fatal("primary acquisition unexpectedly reported alreadyRunning")
	}
	defer windows.CloseHandle(mux)
	defer windows.CloseHandle(ev)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	activated := make(chan struct{}, 2)
	go watchActivateEvent(ctx, ev, func() {
		activated <- struct{}{}
	})

	mux2, ev2, already2, err2 := acquireSingleInstanceNamed(muxName, evName)
	if err2 != nil {
		t.Fatalf("secondary acquire returned error: %v", err2)
	}
	if !already2 || mux2 != 0 || ev2 != 0 {
		t.Fatalf("expected alreadyRunning=true with zero handles, got already=%v mux=%v ev=%v", already2, mux2, ev2)
	}
	if !signalSingleInstanceEvent(evName) {
		t.Fatal("signalSingleInstanceEvent returned false")
	}

	select {
	case <-activated:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for single-instance activation callback")
	}
}
