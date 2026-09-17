# Sprint Change Proposal — Write Pipeline Stall on Invalid Payloads

| Field | Value |
|-------|-------|
| Date | 2026-09-17 |
| Author | PO (Sarah) |
| Trigger | Production incident on v1.0.0 |
| Status | **Approved by the maintainer on 2026-09-17** |
| Mode | Batch (YOLO) |

## 1. Identified Issue Summary

A single MQTT message that PostgreSQL refuses to store stops the whole application from persisting anything, permanently, and a restart does not help.

Observed in production:

```
level=ERROR msg="failed to insert message" component=database event=write_failure operation=insert topic=zigbee2mqtt/bridge/definitions payload_size=283574 duration_us=1213 error="ERROR: unsupported Unicode escape sequence (SQLSTATE 22P05)"
```

The mechanism:

1. `jsonb` refuses the JSON escape `\u0000`: a PostgreSQL `text` value cannot contain the NUL character, even though JSON allows it. `zigbee2mqtt/bridge/definitions` (283 KB, the description of every supported device model) contains at least one.
2. `insertWithRetry` (`cmd/mqtt2bdd/main.go:62`) retries **every** insert error every 10 seconds, forever. That loop was designed for database outages; here the error is permanent, so the same message is rejected indefinitely.
3. `dbWriterLoop` is the only consumer of the buffer. Writes stop, the 1,000-slot buffer fills, then the MQTT handler blocks.
4. `bridge/definitions` is a retained message. The broker re-delivers it on every new subscription, so a restart reproduces the stall immediately.
5. Side effect: `InsertMessage` sets `connected=false` on any error (`internal/database/queries.go:31`), so the health entry reports `db_status=disconnected` while the database is answering.

**Root cause.** The code never implemented a distinction the architecture already makes. `docs/architecture/error-handling-strategy.md` defines two categories: §2 *Transient (Auto-Retry)* and §3 *Data Integrity (Log and Skip)*, the latter explicitly listing "Invalid JSON payload". Stories 1.8, 1.9 and 2.3 implemented only §2. Section §3 was never turned into an acceptance criterion, so no QA gate could catch the gap.

## 2. Evidence — What PostgreSQL Actually Rejects

A probe inserted 34 crafted payloads as `json.RawMessage` into a `JSONB NOT NULL` column, using the project's own driver (`pgx/v5` v5.7.2, which forwards `json.RawMessage` unvalidated) against the dev database (PostgreSQL 15.17, `server_encoding=UTF8`). The table below also records the result of Go's `json.Valid` for each payload.

| Payload | `json.Valid` | PostgreSQL result |
|---------|--------------|-------------------|
| `\u0000` escape (in a value or in a key) | true | **22P05** unsupported Unicode escape sequence |
| `\\u0000` (escaped backslash, literal text) | true | OK, stored unchanged |
| Lone high surrogate `\ud83d`, lone low `\ude00`, reversed pair | true | **22P02** Unicode low surrogate must follow a high surrogate |
| Valid surrogate pair `\ud83d\ude00` | true | OK |
| Noncharacter `\uFFFF`, escaped control `\u0001` | true | OK |
| Raw invalid UTF-8 (`0xff`, CESU-8 surrogate `ED A0 BD`, overlong `C0 AF`) | **true** | **22021** invalid byte sequence for encoding "UTF8" |
| Raw NUL byte, raw tab inside a string, UTF-8 BOM, `NaN`, trailing comma, truncated JSON | false | 22021 / 22P02 |
| Empty payload (zero bytes) — what Paho delivers for an empty MQTT message | false | **22P02** |
| Plain text (`online`) | false | 22P02 |
| `nil` payload | false | **23502** not-null violation |
| Number out of `numeric` range (`1e1000000`, `1e-1000000`, `1e131072`) | **true** | **22003** value overflows numeric format |
| 20,000-digit integer, nesting depth 9,999 | true | OK |
| Nesting depth 100,000 | false | **54001** stack depth limit exceeded |
| Duplicate keys, bare number, bare string, `null` | true | OK (`jsonb` keeps the last duplicate) |

Conclusions:

- Every content-related rejection falls into SQLSTATE class **22** (data exception), **23** (integrity constraint violation) or **54** (program limit exceeded).
- `json.Valid` is a useful local pre-check, but it is not sufficient on its own. Four families pass it and still fail on the server: `\u0000`, unpaired surrogates, invalid UTF-8 and numeric overflow.
- Three of those four families can be repaired without guesswork. Numeric overflow cannot: rewriting a number would be inventing data.
- Empty MQTT payloads are ordinary protocol traffic (clearing a retained message). Today they stall the pipeline too.

## 3. Change Checklist Record

### Section 1 — Trigger & Context

- [x] Triggering story: none in progress. The defect ships in v1.0.0 and originates in Stories 1.8, 1.9 and 2.3.
- [x] Issue type: missing requirement. The architecture anticipated it (§3), but the PRD and the stories never carried it.
- [x] Initial impact: total loss of persistence in production for as long as the offending retained message exists.
- [x] Evidence: production log line above, code reading, and the probe in §2.

### Section 2 — Epic Impact

- [x] Current epic: none open. Epics 1–4 are done and released.
- [x] Completed epics: no rollback. Epic 2's retry loop is correct for transient failures and must stay.
- [x] Future epics: none planned. **A new Epic 5 is required.**
- [x] Summary: one new brownfield epic with three stories, split across two releases.

### Section 3 — Artifact Conflicts

- [x] PRD: FR1 (subscribe to everything), FR6 (no loss during outages) and FR8 (configuration list) need amending. Two new requirements are needed: reject what cannot be stored, and repair what can be.
- [x] Architecture: `error-handling-strategy.md` §2, §3 and summary; `logging-standards.md` event vocabulary, `operation` rule and domain fields; `components.md` configuration struct; `core-workflows.md` error summary table.
- [N/A] Frontend spec: no frontend.
- [x] Other artifacts: `README.md`, `.env.prod.example` and `docker-compose.prod.yml` are updated inside the stories, as acceptance criteria.

### Section 4 — Path Forward

- [x] **Option 1, direct adjustment (selected):** add Epic 5. The change is local to `internal/database`, `internal/config` and `cmd/mqtt2bdd`. It needs no schema change and no new dependency.
- [x] Option 2, rollback: rejected. Nothing to revert; the retry loop is needed.
- [x] Option 3, MVP re-scope: not needed. This strengthens the MVP goal "autonomous, zero-maintenance service" without changing it.

## 4. Recommended Path & Release Plan

| Release | Stories | Why this version number |
|---------|---------|-------------------------|
| **v1.0.1** (urgent) | 5.1, 5.2 | Bug fixes only: no new configuration and no interface change. 5.1 alone unblocks production; 5.2 then turns most of today's rejections back into stored rows. |
| **v1.1.0** | 5.3 | Adds a configuration variable, which is a backward-compatible feature. |

5.1 must merge before 5.2: 5.2's repair step runs ahead of the pre-insert checks that 5.1 introduces, and anything 5.2 cannot repair relies on 5.1's rejection.

**Workaround until v1.0.1:** none on the application side. The retained message is republished by zigbee2mqtt, so it cannot be cleared durably.

## 5. PRD MVP Impact

The goals are unchanged. The promise "no message loss" in FR6 was always scoped to outages, and it stays true there. What changes is explicit: a message whose content the database can never store is now discarded and logged, instead of silently taking every later message down with it.

## 6. Proposed Edits — `docs/prd.md`

### 6.1 Change Log — add row

```
| 2026-09-17 | 0.4.0   | Epic 5 added - write pipeline hardening; FR1, FR6, FR8 amended; FR11, FR12 added | PO (Sarah)      |
```

### 6.2 Functional Requirements

**FR1** — from:

> The application must subscribe to all MQTT topics using wildcard (`#`) on a configured MQTT broker and receive messages in real-time

to:

> The application must subscribe to all MQTT topics using wildcard (`#`) on a configured MQTT broker and receive messages in real-time, discarding before buffering any message whose topic matches an optional, operator-supplied list of MQTT topic filters

**FR6** — from:

> The application must buffer incoming MQTT messages in memory (capacity: 1000 messages) to handle temporary database outages without message loss

to:

> The application must buffer incoming MQTT messages in memory (capacity: 1000 messages) to handle temporary database outages without message loss. This guarantee covers failures of the database, not of a message: a payload the database can never accept is handled by FR11

**FR8** — from:

> (MQTT host/port/credentials, PostgreSQL connection string, log level)

to:

> (MQTT host/port/credentials, PostgreSQL connection string, log level, buffer size, topic exclusion list)

**FR11** (new):

> **FR11:** A single message must never stall the pipeline. A payload the database can never accept — one that is not valid JSON, or one the database rejects with a SQLSTATE of class 22 (data exception), 23 (integrity constraint violation) or 54 (program limit exceeded) — must be discarded immediately and logged at ERROR. Every other write failure keeps being retried as per FR5. An empty payload, which MQTT uses to clear a retained message, is skipped without being treated as an error

**FR12** (new):

> **FR12:** Before insertion, the application must repair the content PostgreSQL would refuse but that can be repaired without guesswork, replacing it with the Unicode replacement character U+FFFD: the JSON escape `\u0000`, unpaired UTF-16 surrogate escapes, and invalid UTF-8 byte sequences. Each repaired message is logged at WARN

### 6.3 Epic List — append

```markdown
### Epic 5: Write Pipeline Hardening

Stop a single invalid message from stalling persistence: classify database write errors into transient and permanent, repair the payload content PostgreSQL refuses when that can be done without guesswork, and let operators exclude topics that carry configuration rather than measurements.
```

### 6.4 New section, appended after Epic 4

````markdown
## Epic 5: Write Pipeline Hardening

**Epic Goal:** Guarantee that no single MQTT message can stop MQTT2BDD from persisting the others. Today one payload PostgreSQL refuses is retried forever by the only database writer, the buffer fills, reception blocks, and — because the offending message is retained by the broker — a restart reproduces the stall at once.

This epic is brownfield and corrective: it was drafted after v1.0.0 shipped, following a production incident on `zigbee2mqtt/bridge/definitions` (SQLSTATE 22P05, `\u0000` in a 283 KB payload). The full analysis, including a probe of what PostgreSQL 15 actually rejects, is recorded in `docs/sprint-change-proposals/2026-09-17-write-pipeline-stall.md`.

### Epic Context

**Existing system context:**

- `insertWithRetry` (`cmd/mqtt2bdd/main.go`) retries every insert error every 10 seconds without limit, and `dbWriterLoop` is the single consumer of the buffer.
- `InsertMessage` (`internal/database/queries.go`) forwards the payload to a `JSONB NOT NULL` column as `json.RawMessage`; `pgx` does not validate it, so PostgreSQL is the only judge of its content.
- `docs/architecture/error-handling-strategy.md` already defines a *Data Integrity (Log and Skip)* category (§3) that was never implemented: this epic implements it.
- The subscription filter is hard-coded to `#`; no configuration exists to narrow it.

**Enhancement details:**

- Story 5.1 separates permanent from transient write errors. Story 5.2 repairs the content that can be repaired. Story 5.3 lets operators exclude topics.
- Release plan: 5.1 and 5.2 ship as **v1.0.1** (bug fixes); 5.3 ships as **v1.1.0** (new configuration variable).

**Compatibility requirements:**

- No schema change. No new Go dependency.
- A valid payload is written exactly as today: same SQL, same retry behaviour on outages, same log entries.
- `MQTT_EXCLUDE_TOPICS` is optional; when unset, the application subscribes to and stores everything, as in v1.0.0.

**Risk mitigation:**

- **Primary risk:** classifying a transient error as permanent silently drops messages that would have been stored after recovery. *Mitigation:* the permanent set is a closed list of three SQLSTATE classes, all describing the value being written. Classes that describe the environment stay retried — connection (08), resources (53), operator intervention (57), system (58), transaction conflicts (40), internal (XX) — and so do class 42 errors (missing privilege, missing table). A class 42 error hits every message identically, and an operator can fix it, as happened on 2026-09-17 with a missing grant.
- **Secondary risk:** stored JSON differs from what the device published. *Mitigation:* only content PostgreSQL would refuse is touched, always with the visible U+FFFD, and every repair is logged. `jsonb` never stored payloads byte-for-byte anyway: it reorders keys, drops duplicate keys and normalises whitespace.
- **Rollback plan:** each story is independently revertible with `git revert`. Reverting 5.1 restores the stall.

### Story 5.1: Stop Retrying Writes the Database Can Never Accept

**As an** operator,
**I want** a message the database can never store to be discarded and logged instead of retried forever,
**so that** one bad payload cannot stop every other message from being persisted.

**Acceptance Criteria:**

1. Write errors are classified inside `internal/database` as either *permanent* or *transient*. The classification is exposed to callers as a sentinel error matchable with `errors.Is` (for example `database.ErrRejected`), so `cmd/mqtt2bdd` never imports `pgconn` or reasons about SQLSTATE codes
2. An error is permanent if and only if it is a `*pgconn.PgError` (found with `errors.As`, so wrapping does not hide it) whose SQLSTATE class is `22`, `23` or `54`. Every other error is transient, including non-PostgreSQL errors, context deadlines, and SQLSTATE classes `08`, `40`, `42`, `53`, `57`, `58` and `XX`
3. Before any database round-trip, a payload that fails `json.Valid` is rejected locally with the same sentinel, without contacting the database
4. A zero-length payload (MQTT's way of clearing a retained message) is skipped before any round-trip: logged at DEBUG with `event=write_skipped` and `reason=empty_payload`, not counted as rejected, not treated as an error by the caller
5. When the error is permanent, `insertWithRetry` returns immediately — no 10-second wait — on both the steady-state path and the shutdown drain path, and the writer moves on to the next buffered message
6. A rejection is logged once, at ERROR, with `event=write_rejected`, `operation=insert`, `topic`, `payload_size`, `duration_us`, `reason` (`invalid_json` for a local rejection, `sqlstate` for a server one), `sqlstate` (server rejections only) and `error`. The payload body is never logged. `write_failure` keeps meaning "this write will be retried" and is no longer emitted for rejections
7. A server-side rejection proves the database answered, so it leaves the client connected: `connected` is set to `true`, never `false`. `lastWriteAt` and `writeCount` are not updated, because nothing was written
8. The database client exposes a monotonic rejection counter, and the `health_check` entry gains a `rejected_last_interval` field computed like `processed_last_interval`
9. Unit tests cover the classification with synthetic `*pgconn.PgError` values — at least `22P05`, `22P02`, `22021`, `22003`, `23502` and `54001` as permanent, and `08006`, `40P01`, `42501`, `42P01`, `53100`, `57P01` and `XX000` as transient — plus a wrapped `PgError`, a plain error and `context.DeadlineExceeded`
10. An integration test publishes, in this order, a non-JSON payload, a payload PostgreSQL rejects server-side (`{"v":1e1000000}`, which passes `json.Valid` and keeps failing after Story 5.2), an empty payload, and a valid payload. The valid row is stored well within the 10-second retry interval, the first three are absent, and two `write_rejected` entries are logged
11. `docs/architecture/error-handling-strategy.md`, `docs/architecture/logging-standards.md` and the README's troubleshooting and data-loss sections describe the new behaviour
12. Verification passes through the containerized toolchain from `dev/` — `gofmt -l .`, `go vet ./...`, `staticcheck ./...`, `go test ./...` — and `./test/run-integration-tests.sh` passes from the repository root

### Story 5.2: Repair Payload Content PostgreSQL Refuses

**As an** operator,
**I want** payload content that PostgreSQL refuses but that can be repaired safely to be repaired rather than discarded,
**so that** a large, legitimate message such as `zigbee2mqtt/bridge/definitions` is stored instead of lost.

**Acceptance Criteria:**

1. `internal/database` repairs the payload before the checks of Story 5.1, replacing each defect with U+FFFD:
   - a JSON `\u0000` escape becomes `\uFFFD`, in keys and values alike, and only when its backslash starts an escape, meaning it is preceded by an even number of backslashes (zero included). `"\\u0000"` is literal text and stays untouched, while `"\\\u0000"` is repaired
   - an unpaired UTF-16 surrogate escape becomes `\uFFFD`: a high surrogate (`\uD800`–`\uDBFF`) not immediately followed by a low surrogate escape, or a low surrogate (`\uDC00`–`\uDFFF`) not immediately preceded by a high one. Valid pairs stay untouched, and hex digits are matched case-insensitively
   - each maximal invalid UTF-8 byte sequence (including CESU-8 encoded surrogates and overlong forms) becomes the UTF-8 encoding of U+FFFD
2. Nothing else is transformed. Escaped control characters `\u0001`–`\u001F`, noncharacters such as `\uFFFF`, and valid surrogate pairs are accepted by PostgreSQL and pass through byte-for-byte. Numeric overflow and structural errors are not repairable and remain rejected by Story 5.1
3. A payload needing no repair is returned unchanged without allocating, verified by a test using `testing.AllocsPerRun`
4. A repaired message is logged once at WARN with `event=payload_sanitized`, no `operation` (it reports a state, not a failed attempt), `topic`, `payload_size`, and one count per repair kind: `nul_escapes`, `lone_surrogates`, `invalid_utf8_sequences`
5. Unit tests cover every repairable case of the proposal's probe table, backslash runs of length 1 to 4 before `u0000`, an escape at the very end of the payload, several defects in one payload, and a payload containing none
6. An integration test publishes `{"a":"x\u0000y"}`, a lone surrogate and a raw `0xff` byte, and finds each stored with U+FFFD in place of the defect
7. The README states that stored JSON may differ from the published payload in this way, and that the database must use the `UTF8` encoding
8. Verification passes as in Story 5.1, AC12

### Story 5.3: Exclude Topics from Persistence

**As an** operator,
**I want** to list MQTT topic filters whose messages are not stored,
**so that** configuration traffic such as `zigbee2mqtt/bridge/#` does not fill `sensor_metrics`.

**Acceptance Criteria:**

1. A new optional environment variable `MQTT_EXCLUDE_TOPICS` holds a comma-separated list of MQTT topic filters. Surrounding whitespace is trimmed, empty entries are ignored, and an unset or empty value excludes nothing
2. Each filter is validated at startup against MQTT 3.1.1 §4.7: `#` only as a whole, final level; `+` only as a whole level; no NUL character. An invalid filter aborts startup with `event=config_load_failed`, naming the offending filter
3. Matching follows MQTT semantics: `+` matches exactly one level; `#` matches the parent level and any number of child levels (`a/#` matches `a`); empty levels are significant; matching is case-sensitive; a topic starting with `$` is not matched by a filter starting with a wildcard
4. An excluded message is dropped in the reception handler before it reaches the buffer, and is not counted by `processed_last_interval`
5. Each exclusion is logged at DEBUG with `event=message_excluded`, `topic` and the matching `filter`; nothing is logged per message at INFO
6. The `config_loaded` entry reports the parsed list as `exclude_topics`
7. The subscription stays `#`: MQTT has no negative subscription, so excluded payloads still cross the network. The README states this
8. `MQTT_EXCLUDE_TOPICS` is documented in the README configuration table (with `zigbee2mqtt/bridge/#` as the example), in `.env.prod.example`, and passed through in `docker-compose.prod.yml`. `docs/architecture/components.md` and `docs/architecture/logging-standards.md` are updated
9. Unit tests cover filter validation, the matching rules above including the examples of MQTT 3.1.1 §4.7, and configuration parsing. An integration test shows that a message on an excluded topic is absent and a message on another topic is stored
10. Verification passes as in Story 5.1, AC12
````

## 7. Proposed Edits — Architecture

### 7.1 `error-handling-strategy.md`

- **§2, "Retry Configuration":** "Max attempts: Infinite for database writes (retry until success)" becomes "Max attempts: infinite for *transient* database write failures; permanent rejections (§3) are never retried".
- **§3, "Data Integrity (Log and Skip)":** the illustrative `HandleMessage` validation code is replaced by the implemented design:
  - the local checks: empty payload skipped, payload repair, `json.Valid`;
  - the permanent SQLSTATE classes (22, 23, 54) and the classes that stay transient, with the reason class 42 is transient;
  - the `ErrRejected` sentinel;
  - `write_rejected` at ERROR, not WARN. A discarded message is data loss, and a WARN is too easy to miss.
- **"Error Handling Strategy Summary":** this closing section is prose, not a table, so it gains an "Isolation" bullet instead: a message the database can never accept is repaired or discarded, never retried.

### 7.2 `core-workflows.md`

- Add rows to its "Error Handling Summary" table for a payload the database can never accept (discarded, **Yes**, that message only), a payload with repairable content (repaired, **No**), an empty payload (skipped, **No**), and "Excluded topic → dropped before buffering → **No** (by configuration)".

### 7.3 `logging-standards.md`

- `event` vocabulary. `database`: add `write_rejected`, `write_skipped` and `payload_sanitized`. `main`: add `message_excluded`.
- `operation` rule: `write_rejected` carries `operation=insert`. `write_skipped`, `payload_sanitized` and `message_excluded` report a state and carry none.
- Domain fields. Add a "Write rejection" row (`reason`, `sqlstate`), a "Payload repair" row (`nul_escapes`, `lone_surrogates`, `invalid_utf8_sequences`) and a "Topic exclusion" row (`filter`). Add `rejected_last_interval` to the health check row and `exclude_topics` to the startup row.

### 7.4 `components.md`

- Configuration Manager: add `ExcludeTopics []string // MQTT topic filters never persisted (default: none)` to the `Config` struct, and the validation rule to "Validation Rules".
- Database Client: add "classify write errors (transient vs permanent) and repair refusable payload content" to its responsibilities.

## 8. Action Plan & Handoff

| # | Action | Owner |
|---|--------|-------|
| 1 | Approve this proposal | Maintainer |
| 2 | Apply §6 to `docs/prd.md` and §7 to `docs/architecture/` | PO (Sarah) |
| 3 | Draft `docs/stories/5.1.story.md`, then 5.2 and 5.3, from Epic 5 | SM |
| 4 | Implement 5.1, then 5.2; QA gate each | Dev, QA |
| 5 | Tag and publish **v1.0.1**; confirm in production that `zigbee2mqtt/bridge/definitions` produces either a stored row or a single `write_rejected`/`payload_sanitized` entry, and that `buffer_used` returns to near zero | Maintainer |
| 6 | Implement 5.3, QA gate, tag **v1.1.0** | Dev, QA, Maintainer |

**Success criteria:** after v1.0.1, the production log no longer shows the same `write_failure` repeating every 10 seconds, `db_status` reports `connected` while the database answers, and `processed_last_interval` is non-zero again.

No PM or Architect replanning is needed: the change stays inside the existing architecture and implements a section of it that was already written.

## 9. Decisions Recorded

| Decision | Choice | By |
|----------|--------|----|
| Epic placement | New Epic 5 (Epic 2 stays closed as released) | Maintainer |
| NUL replacement | U+FFFD rather than deletion, so the repair stays visible | Maintainer |
| Release split | 5.1 + 5.2 → v1.0.1, 5.3 → v1.1.0 | Maintainer |
| Permanent classes | 22, 23, 54 (22 alone would still let a `nil` payload's 23502 or a nesting 54001 stall the pipeline) | PO, approved by maintainer |
| Repair scope | Extended beyond `\u0000` to unpaired surrogates and invalid UTF-8, the other two repairable families found by the probe | PO, approved by maintainer |
| Empty payloads | Skipped at DEBUG, not rejected at ERROR, because they are normal MQTT traffic | PO, approved by maintainer |
