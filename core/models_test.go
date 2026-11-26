package core

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestValidateWireGuardPublicKeyRejectsLowOrderAndAcceptsValid(t *testing.T) {
	zeroKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := ValidateWireGuardPublicKey(zeroKey); err == nil {
		t.Fatal("expected all-zero X25519 public key to be rejected")
	}

	one := make([]byte, 32)
	one[0] = 1
	if err := ValidateWireGuardPublicKey(base64.StdEncoding.EncodeToString(one)); err == nil {
		t.Fatal("expected low-order point u=1 to be rejected")
	}

	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	validKey := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())
	if err := ValidateWireGuardPublicKey(validKey); err != nil {
		t.Fatalf("expected valid X25519 public key to pass, got %v", err)
	}
}

func TestCandidateValidateRejectsLoopbackAndLinkLocal(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:4000", "169.254.1.10:4000", "224.0.0.1:4000", "0.0.0.0:4000", "255.255.255.255:9", "0.0.0.1:9", "240.0.0.1:9", "[2001:db8::1]:9"} {
		c := Candidate{Kind: CandidateHost, Transport: TransportUDP, Address: addr}
		if err := c.Validate(); err == nil {
			t.Fatalf("expected candidate %s to be rejected", addr)
		}
	}
	c := Candidate{Kind: CandidateHost, Transport: TransportUDP, Address: "198.51.100.10:4000"}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected routable unicast candidate to pass, got %v", err)
	}
}
