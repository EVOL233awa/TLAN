package network

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/private-radminlan/lanmesh/core"
)

func TestRelaySnapshotOmitsSecretAndDisconnectDeviceClosesOnlyTarget(t *testing.T) {
	broker := NewRelayBroker(1024)
	first, err := broker.CreateSession(core.DeviceID("device-a"), core.DeviceID("device-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := broker.CreateSession(core.DeviceID("device-c"), core.DeviceID("device-d"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	a, err := broker.Attach(first.TicketFor(core.DeviceID("device-a")), core.DeviceID("device-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := broker.Attach(first.TicketFor(core.DeviceID("device-b")), core.DeviceID("device-b"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := broker.Attach(second.TicketFor(core.DeviceID("device-c")), core.DeviceID("device-c"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := broker.Attach(second.TicketFor(core.DeviceID("device-d")), core.DeviceID("device-d"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()
	defer c.Close()
	defer d.Close()

	snapshot := broker.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("snapshot length = %d, want 2", len(snapshot))
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || containsAny(string(raw), first.Secret, second.Secret) {
		t.Fatalf("snapshot leaked a relay secret: %s", raw)
	}
	if broker.DisconnectDevice(core.DeviceID("device-a")) != 1 {
		t.Fatal("disconnect did not close exactly one session")
	}
	if err := a.Send([]byte("closed")); !errors.Is(err, ErrRelayClosed) {
		t.Fatalf("disconnected device send error = %v", err)
	}
	if err := c.Send([]byte("still-open")); err != nil {
		t.Fatal(err)
	}
	message, err := d.Receive()
	if err != nil || string(message.Data) != "still-open" {
		t.Fatalf("unrelated session message = %q, err=%v", message.Data, err)
	}
}

func TestRelayAttachTakeoverClosesOldConnectionAndForwardsNew(t *testing.T) {
	broker := NewRelayBroker(1024)
	ticket, err := broker.CreateSession(core.DeviceID("device-a"), core.DeviceID("device-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	oldA, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-a")), core.DeviceID("device-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-b")), core.DeviceID("device-b"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	newA, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-a")), core.DeviceID("device-a"))
	if err != nil {
		t.Fatalf("re-attach takeover failed: %v", err)
	}
	defer newA.Close()

	if err := oldA.Send([]byte("stale")); !errors.Is(err, ErrRelayClosed) {
		t.Fatalf("old connection send error = %v, want ErrRelayClosed", err)
	}
	if err := newA.Send([]byte("fresh")); err != nil {
		t.Fatalf("new connection send error = %v", err)
	}
	msg, err := b.Receive()
	if err != nil || string(msg.Data) != "fresh" {
		t.Fatalf("received = %q, err = %v", msg.Data, err)
	}
}

func TestRelayServerTakeoverKeepsSessionAndPeerAlive(t *testing.T) {
	broker := NewRelayBroker(1024)
	defer broker.Close()
	ticket, err := broker.CreateSession(core.DeviceID("device-a"), core.DeviceID("device-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	dial := func(device core.DeviceID) (net.Conn, <-chan error) {
		clientConn, serverConn := net.Pipe()
		srv := &RelayServer{AllowedDevice: device, Broker: broker}
		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.ServeConn(serverConn)
		}()
		if err := json.NewEncoder(clientConn).Encode(RelayHandshake{Ticket: ticket.TicketFor(device), Device: device}); err != nil {
			t.Fatal(err)
		}
		line, err := readHandshakeLine(clientConn, 4096)
		if err != nil {
			t.Fatal(err)
		}
		var ans relayHandshakeResponse
		if err := json.Unmarshal(line, &ans); err != nil || !ans.OK {
			t.Fatalf("handshake failed: ans=%+v err=%v", ans, err)
		}
		return clientConn, errCh
	}

	oldA, oldAErr := dial(core.DeviceID("device-a"))
	bConn, bErr := dial(core.DeviceID("device-b"))
	defer bConn.Close()

	// Peer A reconnects with the same ticket; old A exits and must NOT kill session or B.
	newA, newAErr := dial(core.DeviceID("device-a"))
	defer newA.Close()
	_ = oldA.Close()

	select {
	case <-oldAErr:
	case <-time.After(2 * time.Second):
		t.Fatal("old server handler did not exit after takeover")
	}
	select {
	case err := <-bErr:
		t.Fatalf("peer B unexpectedly disconnected on A takeover: %v", err)
	case err := <-newAErr:
		t.Fatalf("new A unexpectedly disconnected after old A exited: %v", err)
	default:
	}

	if err := writeRelayPacket(newA, []byte("hello-from-new-a"), 4096); err != nil {
		t.Fatalf("write from new A: %v", err)
	}
	gotB, err := readRelayPacket(bConn, 4096)
	if err != nil || string(gotB) != "hello-from-new-a" {
		t.Fatalf("B received = %q, err = %v", gotB, err)
	}
	if err := writeRelayPacket(bConn, []byte("hello-from-b"), 4096); err != nil {
		t.Fatalf("write from B: %v", err)
	}
	gotA, err := readRelayPacket(newA, 4096)
	if err != nil || string(gotA) != "hello-from-b" {
		t.Fatalf("new A received = %q, err = %v", gotA, err)
	}
	_ = io.Discard
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && (len(value) >= len(needle)) {
			for i := 0; i+len(needle) <= len(value); i++ {
				if value[i:i+len(needle)] == needle {
					return true
				}
			}
		}
	}
	return false
}
