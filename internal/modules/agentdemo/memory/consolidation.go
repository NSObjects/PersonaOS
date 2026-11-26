package memory

import (
	"context"
	"time"
)

const (
	// MaxCoreMemoryLength bounds each natural-language memory section in runes.
	MaxCoreMemoryLength = 600
	// MaxPendingBatchSize is a storage safety limit, not a trigger by turn count.
	MaxPendingBatchSize = 64
	// ConsolidationThreshold triggers consolidation on new user content.
	ConsolidationThreshold = 3000
	// ConsolidationIdleMinimum avoids model calls for tiny idle exchanges.
	ConsolidationIdleMinimum = 500
	// ConsolidationIdleAge is the minimum quiet period for a smaller batch.
	ConsolidationIdleAge = 5 * time.Minute
)

// CoreMemory contains stable preferences and dynamic relationship context.
type CoreMemory struct {
	Stable  string
	Dynamic string
}

// CoreState identifies the current memory revision and attempted queue prefix.
type CoreState struct {
	CoreMemory
	Version   uint64
	ThroughID int64
}

// PendingExchange references one durably archived complete exchange.
type PendingExchange struct {
	ID int64
	Exchange
}

// PendingBatch is an ordered snapshot. The caller may process a prefix only.
type PendingBatch struct {
	Core      CoreState
	Exchanges []PendingExchange
}

// ConsolidationStore archives exchanges and claims each selected prefix once.
// ClaimPending advances the attempt cursor before any model call. PublishCore
// changes the summary only if the claim revision is still current.
type ConsolidationStore interface {
	AppendPending(context.Context, Exchange) error
	ReadPending(context.Context, Scope, int) (PendingBatch, error)
	ClaimPending(context.Context, Scope, uint64, []int64) error
	PublishCore(context.Context, Scope, uint64, CoreMemory) error
}

// PendingScope describes a scope whose unacknowledged user content is ready.
type PendingScope struct {
	Scope
	LastPendingAt time.Time
	UserChars     int64
}

// PendingScopeReader discovers eligible pending work using a bounded keyset page.
// The zero scope starts a fresh scan. now determines idle eligibility.
type PendingScopeReader interface {
	ListReadyScopes(context.Context, Scope, time.Time, int) ([]PendingScope, error)
}

// CoreReader loads only the published summary, not queued or archived dialog.
type CoreReader interface {
	ReadCore(context.Context, Scope) (CoreMemory, error)
}
