//go:build windows && !installer_payload

package main

import (
	"embed"
	"io/fs"
)

var embeddedPayload embed.FS

func payloadFS() fs.FS {
	return embeddedPayload
}
