// Package contextartifact exposes frozen research artifacts to offline tools.
package contextartifact

import (
	_ "embed"
)

// PilotSchemaSHA256 is the reviewed digest of pilot-schema.json.
const PilotSchemaSHA256 = "1a63ab0a3f3e377e09b54c8993bc0d2bc8fb3fe3512fc401c10bc0f188ee2e41"

// FinalReceiptSchemaSHA256 is the reviewed raw digest of final-receipt-schema-v1.json.
const FinalReceiptSchemaSHA256 = "074277e7b6f9ea30b6659ff7ea8d2d72433219b086c0fc3ef731a5679852c5bf"

// FinalSourceRegistrySHA256 is the reviewed raw digest of final-source-registry-v1.json.
const FinalSourceRegistrySHA256 = "b5329538427ee98c6aff4027506622fd1b4c4b5920d2acd9bc470f2da9e2059c"

// PilotSchema contains the authoritative adjacent schema bytes.
//
//go:embed pilot-schema.json
var PilotSchema []byte

// FinalReceiptSchema contains the exact authoritative final receipt schema bytes.
//
//go:embed final-receipt-schema-v1.json
var FinalReceiptSchema []byte

// FinalSourceRegistry contains the exact reviewed source registry bytes.
//
//go:embed final-source-registry-v1.json
var FinalSourceRegistry []byte
