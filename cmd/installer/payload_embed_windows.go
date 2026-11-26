//go:build windows && installer_payload

package main

import (
	"embed"
	"io/fs"
)

//go:embed payload/*
var embeddedPayload embed.FS

func payloadFS() fs.FS {
	sub, err := fs.Sub(embeddedPayload, "payload")
	if err != nil {
		return embeddedPayload
	}
	return sub
}
