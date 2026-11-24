# PersonaOS Demo Memory

This is a minimal **opt-in** preference memory for the character chat Demo. It is not an account-wide memory service, a child profile system, or a production-grade durable task queue.

## Behavior

- Existing browser conversation history remains the short-term context.
- On `/agent-demo`, memory is **off by default**. The operator may explicitly enable it for a test conversation. The toggle resets when switching device types.
- For a remembered turn, the server derives the user ID from the authenticated request, and the character ID from the *active device-type snapshot*. The browser cannot provide or override those identity keys.
- The main Agent retrieves at most eight indexed preference facts with a 250 ms database deadline; read failures do not prevent a reply. The Eino agent receives those facts as untrusted reference data.
- After the real LLM reply is ready, a bounded in-process job writes the user and assistant messages to `conversation_messages`, then asks the Eino model to extract at most three explicitly stated preferences and upserts them into `memory_facts`.
- Extraction only allows a small list of non-sensitive preference keys, rejects numeric identifiers and email-like values, and never extracts assistant statements.

The new tables are created through the existing startup migration. No Redis server, vector index, extra Agent runtime, new publishing flow or device-model constants are required.

## Demo limitations

In-process jobs are **best-effort**: a process crash or full worker pool can lose a turn. The response exposes `memory_queued`, which means only that the background job was accepted, not that it has been persisted. A durable queue/retry/retention service is deliberately outside this Demo scope.

This Demo associates memory with the **logged-in admin user + character**, not with a real child. **Do not input actual children's personal information for demonstrations.** The product must introduce authenticated device-user identity, parental consent, retention and erasure controls before real-child use.

Only limited, deliberately non-sensitive preferences are eligible for extraction. The interface does not expose raw memory tables or an API to access another user's memory.

## Acceptance

- Memory disabled: the existing Eino chat works without any memory reads/writes.
- Memory enabled: recent facts enter the next prompt, and completed turns are queued **after** reply generation.
- Two users or two different characters cannot retrieve one another's memory facts.
- Background extraction failures do not delay or invalidate successful chat replies.
- Real MySQL integration verifies table creation, saved user/assistant messages, scope isolation, and fact upserts.
