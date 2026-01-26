package config

import "time"

// Batch processing status constants
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Batch processing action constants
const (
	ActionValidated  = "validated"
	ActionRegistered = "registered"
)

// Change level constants (for semantic versioning)
const (
	ChangeLevelPatch   = "patch"   // Metadata-only changes
	ChangeLevelMinor   = "minor"   // Backward-compatible structural changes
	ChangeLevelMajor   = "major"   // Breaking changes
	ChangeLevelInitial = "initial" // New schema
)

// Output format constants
const (
	FormatJSON     = "json"
	FormatTable    = "table"
	FormatSummary  = "summary"
	FormatMarkdown = "markdown"
)

// Timeout constants for operations
const (
	DefaultOperationTimeout = 30 * time.Second  // For single operations
	BatchOperationTimeout   = 5 * time.Minute   // For batch operations
)

// Initial version for new schemas
const InitialVersion = "1.0.0"
