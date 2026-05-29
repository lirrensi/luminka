// FILE: luminka/version.go
// PURPOSE: Expose runtime and protocol version metadata for CLI and websocket surfaces.
// OWNS: Luminka runtime version constants and app version defaulting.
// EXPORTS: RuntimeVersion, ProtocolVersion
// DOCS: CHANGELOG.md

package luminka

const (
	RuntimeVersion    = "4.0.0"
	ProtocolVersion   = "2"
	defaultAppVersion = "dev"
)
