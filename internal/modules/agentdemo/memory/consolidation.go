package memory

import "context"

const (
	// MaxCoreMemoryLength bounds each natural-language memory section in runes.
	MaxCoreMemoryLength = 600
	// MaxPendingBatchSize is a storage safety limit, not a trigger by turn count.
	MaxPendingBatchSize = 64
)

// CoreMemory contains stable preferences and dynamic relationship context.
type CoreMemory struct {
	Stable  string
	Dynamic string
}

// CoreState identifies one committed revision of the consolidated memory.
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

// ConsolidationStore provides atomic archive admission and acknowledgement.
// An unsuccessful consolidation must leave pending exchanges untouched.
type ConsolidationStore interface {
	AppendPending(context.Context, Exchange) error
	ReadPending(context.Context, Scope, int) (PendingBatch, error)
	CommitPending(context.Context, Scope, uint64, []int64, CoreMemory) error
}
