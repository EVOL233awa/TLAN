package core

import (
	"strings"
	"testing"
)

func TestParseMajorMinor(t *testing.T) {
	cases := []struct {
		raw   string
		major int
		minor int
		ok    bool
	}{
		{raw: "1.1.0", major: 1, minor: 1, ok: true},
		{raw: "v1.1.3", major: 1, minor: 1, ok: true},
		{raw: "V2.0.1-beta.1", major: 2, minor: 0, ok: true},
		{raw: "1.1", major: 1, minor: 1, ok: true},
		{raw: "", ok: false},
		{raw: "latest", ok: false},
		{raw: "1.x.0", ok: false},
	}
	for _, tc := range cases {
		maj, min, err := ParseMajorMinor(tc.raw)
		if tc.ok && (err != nil || maj != tc.major || min != tc.minor) {
			t.Fatalf("ParseMajorMinor(%q) = (%d, %d, %v), want (%d, %d, nil)", tc.raw, maj, min, err, tc.major, tc.minor)
		}
		if !tc.ok && err == nil {
			t.Fatalf("ParseMajorMinor(%q) expected error, got (%d, %d)", tc.raw, maj, min)
		}
	}
}

func TestMajorMinorCompatible(t *testing.T) {
	if err := MajorMinorCompatible("1.1.0", "v1.1.9"); err != nil {
		t.Fatalf("expected compatible versions, got: %v", err)
	}
	for _, pair := range [][2]string{
		{"1.1.0", "v1.0.0"},
		{"1.1.0", "v1.2.0"},
		{"1.1.0", "v2.1.0"},
		{"1.1.0", ""},
	} {
		err := MajorMinorCompatible(pair[0], pair[1])
		if err == nil || !strings.Contains(err.Error(), "version_mismatch") {
			t.Fatalf("MajorMinorCompatible(%q, %q) = %v, want version_mismatch", pair[0], pair[1], err)
		}
	}
}
