package network

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestDecodeSTUNAddressIPv4(t *testing.T) {
	txid := [12]byte{}
	value := []byte{0, 1, 0x1f, 0x90, 192, 0, 2, 44} // 8080, 192.0.2.44
	got, err := decodeSTUNAddress(value, false, txid)
	if err != nil {
		t.Fatal(err)
	}
	if want := netip.MustParseAddrPort("192.0.2.44:8080"); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestDecodeSTUNXORMappedIPv4(t *testing.T) {
	txid := [12]byte{1, 2, 3}
	want := netip.MustParseAddrPort("198.51.100.7:3478")
	value := make([]byte, 8)
	value[1] = 1
	binary.BigEndian.PutUint16(value[2:4], want.Port()^uint16(stunMagicCookie>>16))
	ip := want.Addr().As4()
	for i := range ip {
		value[4+i] = ip[i] ^ [4]byte{0x21, 0x12, 0xA4, 0x42}[i]
	}
	got, err := decodeSTUNAddress(value, true, txid)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParseSTUNBindingResponsePrefersXORMapped(t *testing.T) {
	txid := [12]byte{10, 11, 12}
	packet := makeSTUNResponse(stunBindingSuccess, txid,
		stunAttribute(stunAttrMappedAddress, []byte{0, 1, 0x13, 0x88, 192, 0, 2, 1}),
		stunAttribute(stunAttrXORMappedAddress, xorAddress(netip.MustParseAddrPort("198.51.100.8:4000"), txid)),
	)
	result, err := parseSTUNBindingResponse(packet, txid, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000})
	if err != nil {
		t.Fatal(err)
	}
	if result.MappedAddress != "198.51.100.8:4000" || !result.SupportsUDP || result.NAT != NATUnknown {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUDPSTUNClientProbeLocalServer(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1500)
		n, addr, readErr := server.ReadFromUDP(buf)
		if readErr != nil {
			done <- readErr
			return
		}
		if n != stunHeaderSize || binary.BigEndian.Uint16(buf) != stunBindingRequest {
			done <- &testError{"invalid STUN request"}
			return
		}
		var txid [12]byte
		copy(txid[:], buf[8:20])
		response := makeSTUNResponse(stunBindingSuccess, txid,
			stunAttribute(stunAttrXORMappedAddress, xorAddress(netip.MustParseAddrPort("203.0.113.9:4567"), txid)),
		)
		_, writeErr := server.WriteToUDP(response, addr)
		done <- writeErr
	}()

	client := &UDPSTUNClient{Timeout: time.Second}
	result, err := client.Probe(context.Background(), server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if result.MappedAddress != "203.0.113.9:4567" || result.ServerAddress != server.LocalAddr().String() {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUDPSTUNClientProbeHonorsContext(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	client := &UDPSTUNClient{Timeout: time.Second}
	_, err = client.Probe(ctx, server.LocalAddr().String())
	if err == nil || !isContextError(err) {
		t.Fatalf("expected context timeout, got %v", err)
	}
}

func TestSTUNGlobalBudgetNotStarvedByTwentySources(t *testing.T) {
	if stunGlobalRatePerSecond < 200 {
		t.Fatalf("stunGlobalRatePerSecond = %d, want >= 200", stunGlobalRatePerSecond)
	}
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeSTUN(ctx, server)
	}()

	// Send 10 STUN requests per source IP across 20 distinct loopback source IPs (200 total).
	var txid [12]byte
	copy(txid[:], "twenty-srcs!")
	req := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(req[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(req[2:4], 0)
	binary.BigEndian.PutUint32(req[4:8], stunMagicCookie)
	copy(req[8:20], txid[:])

	buf := make([]byte, 128)
	serverAddr := server.LocalAddr().(*net.UDPAddr)
	for i := 1; i <= 20; i++ {
		client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, byte(i)), Port: 0})
		if err != nil {
			t.Skipf("distinct loopback bind unsupported in this environment: %v", err)
		}
		for attempt := 0; attempt < 10; attempt++ {
			if _, err := client.WriteToUDP(req, serverAddr); err != nil {
				client.Close()
				t.Fatal(err)
			}
			_ = client.SetReadDeadline(time.Now().Add(time.Second))
			n, _, err := client.ReadFromUDP(buf)
			if err != nil || n < 32 {
				client.Close()
				t.Fatalf("source 127.0.0.%d request %d failed (dropped by global budget?): n=%d err=%v", i, attempt+1, n, err)
			}
		}
		client.Close()
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeSTUN error: %v", err)
	}
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func isContextError(err error) bool {
	return err == context.DeadlineExceeded || err == context.Canceled
}

func stunAttribute(kind uint16, value []byte) []byte {
	padded := (len(value) + 3) &^ 3
	attr := make([]byte, 4+padded)
	binary.BigEndian.PutUint16(attr[0:2], kind)
	binary.BigEndian.PutUint16(attr[2:4], uint16(len(value)))
	copy(attr[4:], value)
	return attr
}

func makeSTUNResponse(kind uint16, txid [12]byte, attrs ...[]byte) []byte {
	length := 0
	for _, attr := range attrs {
		length += len(attr)
	}
	packet := make([]byte, stunHeaderSize+length)
	binary.BigEndian.PutUint16(packet[0:2], kind)
	binary.BigEndian.PutUint16(packet[2:4], uint16(length))
	binary.BigEndian.PutUint32(packet[4:8], stunMagicCookie)
	copy(packet[8:20], txid[:])
	offset := stunHeaderSize
	for _, attr := range attrs {
		copy(packet[offset:], attr)
		offset += len(attr)
	}
	return packet
}

func xorAddress(addr netip.AddrPort, txid [12]byte) []byte {
	value := make([]byte, 8)
	value[1] = 1
	binary.BigEndian.PutUint16(value[2:4], addr.Port()^uint16(stunMagicCookie>>16))
	ip := addr.Addr().As4()
	mask := [4]byte{0x21, 0x12, 0xA4, 0x42}
	for i := range ip {
		value[4+i] = ip[i] ^ mask[i]
	}
	return value
}
