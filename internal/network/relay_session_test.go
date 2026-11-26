package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/private-radminlan/lanmesh/core"
)

// TestRelayClosePropagatesToPeer pins issue #29: closing one endpoint must
// wake the peer with ErrRelayClosed instead of leaving Receive blocked until
// the stream read timeout.
func TestRelayClosePropagatesToPeer(t *testing.T) {
	broker := NewRelayBrokerWithLimits(RelayLimits{QueueDepth: 8, MaxSessions: 4, IdleTimeout: time.Minute})
	defer broker.Close()
	ticket, err := broker.CreateSession(core.DeviceID("device-a"), core.DeviceID("device-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	a, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-a")), core.DeviceID("device-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-b")), core.DeviceID("device-b"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := b.ReceiveContext(ctx); !errors.Is(err, ErrRelayClosed) {
		t.Fatalf("peer receive after close = %v, want ErrRelayClosed", err)
	}
}

// TestRelayTicketCannotAttachAsOtherEndpoint pins issue #29: a ticket issued
// to one endpoint must not let that endpoint take over the other's stream.
func TestRelayTicketCannotAttachAsOtherEndpoint(t *testing.T) {
	broker := NewRelayBroker(1024)
	defer broker.Close()
	ticket, err := broker.CreateSession(core.DeviceID("device-a"), core.DeviceID("device-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	aTicket := ticket.TicketFor(core.DeviceID("device-a"))
	a, err := broker.Attach(aTicket, core.DeviceID("device-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := broker.Attach(ticket.TicketFor(core.DeviceID("device-b")), core.DeviceID("device-b"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if _, err := broker.Attach(aTicket, core.DeviceID("device-b")); !errors.Is(err, ErrRelayUnauthorized) {
		t.Fatalf("cross-endpoint attach = %v, want ErrRelayUnauthorized", err)
	}
	// The impersonation attempt must not have closed the real target.
	if err := b.Send([]byte("alive")); err != nil {
		t.Fatalf("target send after impersonation attempt = %v", err)
	}
	message, err := a.Receive()
	if err != nil || string(message.Data) != "alive" {
		t.Fatalf("received = %q, err = %v", message.Data, err)
	}
}
