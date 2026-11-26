//go:build windows

package main

import "testing"

func TestResolveCloseBehaviorDefaultsToTray(t *testing.T) {
	cases := []struct {
		name          string
		closeToTray   bool
		explicitExit  bool
		trayAvailable bool
		want          closeBehavior
	}{
		{name: "default close hides to notification area", closeToTray: false, trayAvailable: true, want: closeBehaviorTray},
		{name: "legacy tray setting stays enabled", closeToTray: true, trayAvailable: true, want: closeBehaviorTray},
		{name: "explicit exit wins over tray", closeToTray: true, explicitExit: true, trayAvailable: true, want: closeBehaviorExit},
		{name: "explicit exit wins over default tray", closeToTray: false, explicitExit: true, trayAvailable: true, want: closeBehaviorExit},
		{name: "default tray unavailable falls back to minimize", closeToTray: false, trayAvailable: false, want: closeBehaviorMinimize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveCloseBehavior(tc.closeToTray, tc.explicitExit, tc.trayAvailable); got != tc.want {
				t.Fatalf("close behavior = %v, want %v", got, tc.want)
			}
		})
	}
}
