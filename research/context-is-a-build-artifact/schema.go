// Package contextartifact exposes frozen research artifacts to offline tools.
package contextartifact

import (
	_ "embed"
)

// PilotSchemaSHA256 is the reviewed digest of pilot-schema.json.
const PilotSchemaSHA256 = "1a63ab0a3f3e377e09b54c8993bc0d2bc8fb3fe3512fc401c10bc0f188ee2e41"

// PilotSchema contains the authoritative adjacent schema bytes.
//
//go:embed pilot-schema.json
var PilotSchema []byte
