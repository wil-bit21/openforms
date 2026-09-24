// Package schemas holds the JSON Schemas for openforms definitions and the
// conformance fixtures shared by the Go server and the TypeScript SDK.
package schemas

import "embed"

// FS contains form.schema.json and workflow.schema.json.
//
//go:embed form.schema.json workflow.schema.json
var FS embed.FS

// Fixtures contains fixtures/visibility.json and fixtures/submission.json.
//
//go:embed fixtures/*.json
var Fixtures embed.FS
