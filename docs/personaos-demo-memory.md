# PersonaOS Demo Memory

## Scope and recall

Memory is opt-in on the Agent Demo page. The authenticated user and the active character define its scope; different device types using the same character share that memory. The browser cannot supply another user's identity.

Recent session context retains complete exchanges within a dynamic 4,000–5,000 Unicode-character window. Long-term memory contains two sections, stable preferences and agreements, and dynamic goals and recent progress, each limited to 600 characters. The Agent reads the published summary through a bounded, on-demand process cache (256 scopes, 60-second TTL). Database lookup failures do not block a normal reply. The current dialog takes precedence over outdated memories.

## Consolidation

A completed opted-in reply queues an asynchronous archive write. The archive and pending references commit together. No memory-model request is made for each individual turn.

The scheduler discovers pending scopes every 15 seconds after a scan. A scope qualifies when unprocessed **user text** reaches 3,000 characters, or reaches 500 characters followed by five minutes of inactivity. Assistant verbosity, elapsed time alone, and empty queues never trigger model calls.

Each selected ordered batch is claimed before model resolution. Its input contains the prior core and up to 5,000 characters of new dialog bodies, prioritizing user statements. A valid changed summary is published with a version check; an unchanged result does not rewrite the summary. Errors end the attempt without automatic retries or changes to the prior core. Original dialog archives remain intact. Only newly eligible work enters the next consolidation.

## Management and migration

On startup, legacy preference facts are transferred transactionally into the bounded summary without model calls or historic replay. Successfully migrated facts leave the legacy table. If a scope cannot fit, migration fails without deleting that scope's original facts.

The memory panel shows stable and dynamic summaries and can clear the current user's memory for the active character. A clear removes the summary, legacy facts and pending references in a transaction, advances the version, and invalidates the cache. Old in-flight consolidation results cannot publish after the clear. Queued local archive writes must finish before clearing. Archived conversation history and existing short-term context are not deleted; new opted-in turns can learn information again.

## Limitations

The Demo uses one scheduler per application process and a local cache. It is best effort: memory_queued indicates acceptance by a bounded in-process writer, not guaranteed persistence. Failed consolidation attempts may omit otherwise useful information. No retries, vector memory, new infrastructure, or historical reprocessing are introduced. Distributed admission and cache invalidation, archive retention and full erasure are outside this Demo.

Only directly stated, non-sensitive preferences and useful relationship context should enter core memory. Do not use actual children's personal information for demonstrations.

## Verification

Tests cover scoped recall, opt-out, fail-open reads, cache invalidation, ordered pending claims, no replay of failed batches, clear/version conflicts, and MySQL migration. The optional live voice test accumulates enough user content through the Agent API before checking consolidated recall in a fresh session.
