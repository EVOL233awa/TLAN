package control

import (
	"fmt"
	"testing"

	"github.com/private-radminlan/lanmesh/core"
)

// TestRelayStreamPerDeviceQuota pins issue #29: one member must not be able to
// exhaust the shared relay stream pool.
func TestRelayStreamPerDeviceQuota(t *testing.T) {
	s := &Server{}
	releases := make([]func(), 0, relayMaxStreamsPerDevice)
	for i := 0; i < relayMaxStreamsPerDevice; i++ {
		release, ok := s.acquireRelayStream(core.DeviceID("device-a"))
		if !ok {
			t.Fatalf("slot %d denied for first device", i)
		}
		releases = append(releases, release)
	}
	if _, ok := s.acquireRelayStream(core.DeviceID("device-a")); ok {
		t.Fatal("per-device quota was not enforced")
	}
	if _, ok := s.acquireRelayStream(core.DeviceID("device-b")); !ok {
		t.Fatal("another device was blocked by the first device's quota")
	}
	for _, release := range releases {
		release()
	}
	if _, ok := s.acquireRelayStream(core.DeviceID("device-a")); !ok {
		t.Fatal("released slots were not returned to the pool")
	}
}

func TestRelayStreamGlobalQuota(t *testing.T) {
	s := &Server{}
	for device := 0; device*relayMaxStreamsPerDevice < relayMaxStreams; device++ {
		for slot := 0; slot < relayMaxStreamsPerDevice; slot++ {
			if _, ok := s.acquireRelayStream(core.DeviceID(fmt.Sprintf("device-%02d", device))); !ok {
				t.Fatalf("device %d slot %d denied before the global limit", device, slot)
			}
		}
	}
	if _, ok := s.acquireRelayStream(core.DeviceID("overflow")); ok {
		t.Fatal("global stream quota was not enforced")
	}
}
