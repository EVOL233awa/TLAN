package control

import (
	"bufio"
	"net"
	"net/http"
	"strings"

	"github.com/private-radminlan/lanmesh/core"
	"github.com/private-radminlan/lanmesh/internal/network"
)

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

const (
	relayMaxStreams          = 2048
	relayMaxStreamsPerDevice = 64
)

func (s *Server) acquireRelayStream(device core.DeviceID) (func(), bool) {
	s.relayStreamMu.Lock()
	defer s.relayStreamMu.Unlock()
	if s.relayStreamTotal >= relayMaxStreams {
		return nil, false
	}
	if s.relayStreams == nil {
		s.relayStreams = make(map[core.DeviceID]int)
	}
	if s.relayStreams[device] >= relayMaxStreamsPerDevice {
		return nil, false
	}
	s.relayStreamTotal++
	s.relayStreams[device]++
	return func() {
		s.relayStreamMu.Lock()
		defer s.relayStreamMu.Unlock()
		s.relayStreamTotal--
		if remaining := s.relayStreams[device] - 1; remaining > 0 {
			s.relayStreams[device] = remaining
		} else {
			delete(s.relayStreams, device)
		}
	}, true
}

// relayStream carries encrypted WireGuard datagrams over the same HTTPS
// origin as the control API. It needs HTTP/1.1 Upgrade support in the proxy.
func (s *Server) relayStream(w http.ResponseWriter, r *http.Request) {
	if s.Relay == nil {
		writeError(w, 503, "relay disabled")
		return
	}
	if r.Method != http.MethodGet || !strings.EqualFold(r.Header.Get("Upgrade"), "lanmesh-relay") {
		writeError(w, 400, "HTTP Upgrade lanmesh-relay required")
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		writeError(w, 505, "HTTP/1.1 required")
		return
	}
	var device core.DeviceID
	if member, ok := memberIdentity(r); ok {
		device = core.DeviceID(member.DeviceID)
	}
	release, ok := s.acquireRelayStream(device)
	if !ok {
		writeError(w, 503, "relay stream capacity reached")
		return
	}
	defer release()
	connection, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if _, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: lanmesh-relay\r\n\r\n"); err != nil {
		connection.Close()
		return
	}
	if err = rw.Flush(); err != nil {
		connection.Close()
		return
	}
	relayServer := &network.RelayServer{Broker: s.Relay, AllowedDevice: device}
	_ = relayServer.ServeConn(&bufferedConn{connection, rw.Reader})
}
