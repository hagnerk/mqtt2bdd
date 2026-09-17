# Logging Standards

This document is the durable contract for the structured-logging vocabulary used across the
application: which fields appear on every entry, which are inherited per component, and the
enumerated values `component`, `event` and `operation` may take.

It refines the **Required Context Fields** list in
[Error Handling Strategy → Logging Standards for Errors](./error-handling-strategy.md#logging-standards-for-errors),
which states *which* fields are required; this document states *which values* they may hold.

Log entries are operational, not contractual — their wording is free to change. The vocabulary
below is what an operator filters and aggregates on, so it is the part that must stay stable, and
any new value belongs in this document as part of the story that introduces it.

## Baseline Fields

Present on **every** entry. `time`, `level` and `msg` come from slog itself; `component` is attached
once per component logger via `logger.WithComponent`.

| Field | Source | Values |
|-------|--------|--------|
| `time` | slog `TimeKey`, rewritten to UTC | ISO 8601 / RFC 3339 with `Z` suffix |
| `level` | slog | `DEBUG` / `INFO` / `WARN` / `ERROR` |
| `msg` | slog | short lowercase-ish phrase, no interpolated values |
| `component` | `.With()` at construction | `main`, `mqtt`, `database` |

Values are carried as **fields, never interpolated into `msg`** — a value spelled into the message
string is a value no log processor can chart or aggregate.

## Inherited Fields

Attached once when the component logger is constructed, never repeated per call.

| Component | Inherited fields |
|-----------|------------------|
| `mqtt` | `broker` |
| `database` | `host`, `database` |
| `main` | — |

## `event` Vocabulary

Exactly one `event` per log call.

| Component | `event` values |
|-----------|----------------|
| `main` | `starting`, `unknown_log_level`, `config_load_failed`, `config_loaded`, `buffer_initialised`, `writer_started`, `health_monitor_started`, `startup_complete`, `startup_aborted`, `buffer_full`, `buffer_high`, `message_dropped`, `message_excluded`, `health_check`, `shutdown_signal`, `flush_complete`, `flush_timeout`, `shutdown_complete` |
| `mqtt` | `connecting`, `connected`, `connect_timeout`, `connect_failed`, `connection_lost`, `reconnecting`, `reconnect_failed`, `reconnected`, `reconnect_cancelled`, `disconnected`, `subscribing`, `subscribed`, `subscribe_failed`, `resubscribe_failed`, `message_received` |
| `database` | `connecting`, `connected`, `connect_failed`, `disconnected`, `write_success`, `write_failure`, `write_duplicate`, `write_rejected`, `write_skipped`, `payload_sanitized`, `topic_truncated`, `pool_stats` |

`message_excluded`, `write_rejected`, `write_skipped` and `payload_sanitized` are specified by Epic 5 (Stories 5.1–5.3) ahead of their implementation.

`unknown_log_level` is emitted from the `logger` package itself but tagged `component=main`, because
it reports on the application's own configuration rather than on the logger as a subsystem.

## `operation` Vocabulary

`operation` is **required on every ERROR or WARN entry that reports a failed attempt**, and names
what was being attempted: `load_config`, `connect`, `reconnect`, `subscribe`, `parse_dsn`, `ping`,
`insert`.

Entries that report a *state* rather than a failed attempt carry **no** `operation`, whatever their
level — there is no operation to name. These are `connection_lost` at ERROR, and `buffer_full`,
`buffer_high`, `message_dropped`, `flush_timeout`, `write_duplicate`, `topic_truncated`,
`payload_sanitized`, `unknown_log_level` at WARN.

`connection_lost` is the single ERROR in that list, and the reason the rule is not phrased "every
ERROR carries an operation": the broker dropped the link on its own, no local call was attempted, so
any `operation` value would be fabricated.

`write_rejected` reports a failed attempt and carries `operation=insert`, like `write_failure`. The two differ in what follows: a `write_failure` is retried, a `write_rejected` message is discarded.

## Domain Fields

| Area | Fields |
|------|--------|
| Message processing (`write_success` / `write_failure` / `write_duplicate`) | `topic`, `payload_size` (bytes), `duration_us` (microseconds — see note below), `timestamp` (RFC 3339, the message's own timestamp; `write_success`/`write_duplicate` only, distinct from the log entry's own `time`), `query` (the literal SQL text executed; `write_success` only, DEBUG level) |
| Write rejection (`write_rejected`) | `topic`, `payload_size` (bytes), `duration_us` (`0` for a local rejection), `reason` (`invalid_json` for a local rejection, `sqlstate` for a server one), `sqlstate` (the five-character code; `reason=sqlstate` only). The payload body is never included |
| Write skip (`write_skipped`, DEBUG only) | `topic`, `reason` (`empty_payload`) |
| Payload repair (`payload_sanitized`) | `topic`, `payload_size` (bytes, before repair), `nul_escapes`, `lone_surrogates`, `invalid_utf8_sequences` (one count per repair kind) |
| Topic exclusion (`message_excluded`, DEBUG only) | `topic`, `filter` (the `MQTT_EXCLUDE_TOPICS` entry that matched) |
| Message reception (`message_received`, DEBUG only) | `topic`, `payload_size` (bytes), `payload` (the raw message payload as a string; DEBUG-only and level-guarded so the conversion cost is not paid at INFO) |
| Sensor name truncation (`topic_truncated`) | `original_length` (rune-count of the sensor name before truncation), `truncated_topic` (the truncated, last-255-rune sensor name actually used for the insert) |
| Reconnection (`reconnecting` / `reconnect_failed`) | `attempt` (1-based reconnect attempt counter) |
| Connection pool (`pool_stats`, DEBUG only) | `acquired_conns`, `idle_conns`, `total_conns`, `max_conns` — the four `pgxpool.Stat()` counters; `max_conns` is the configured ceiling (`internal/database/client.go`'s `maxConns`), not a live count |
| Log level validation (`unknown_log_level`) | `level` (the raw, unrecognized `LOG_LEVEL` value the operator supplied — distinct from `log_level`/`effective_log_level` below, which are always valid) |
| Health check (`health_check`) | `mqtt_status`, `db_status`, `db_last_write_age_s` (`-1` ⇒ no successful write since startup), `buffer_used`, `buffer_capacity`, `buffer_utilization_percent`, `processed_last_interval`, `rejected_last_interval` (messages discarded by `write_rejected` since the previous entry) |
| Buffer pressure (`buffer_full` / `buffer_high`) | `buffer_size` (`buffer_full` only — the configured channel capacity, `cfg.BufferSize`; also emitted at startup on `config_loaded`, see below — the field is genuinely shared across both areas, not mis-filed in one), `buffer_used`, `buffer_capacity`, `buffer_utilization_percent` (`buffer_high` only) |
| Startup / shutdown | `version`, `log_level`, `effective_log_level`, `buffer_size` (`config_loaded` — same field as the `buffer_full` row above), `exclude_topics` (`config_loaded` — the parsed `MQTT_EXCLUDE_TOPICS` list), `buffer_capacity`, `interval_s`, `signal`, `count`, `buffered`, `failed_component` |

`failed_component` is deliberately not `component`: an entry naming a culprit is still emitted by
`main`, and `component` must keep identifying the emitter.

`log_level` is the value the operator requested; `effective_log_level` is the level actually in
force after an unrecognised value has resolved to INFO. Both appear on `config_loaded`, so a
mismatch is diagnosable from that single entry.

**`duration_us` (renamed from `duration_ms`):** `time.Duration.Milliseconds()` truncates toward
zero, so any write completing in under 1 ms reported a metric-blinding `duration_ms=0`,
indistinguishable from a genuinely instantaneous write. `duration_us` uses
`elapsed.Microseconds()` instead — still an `int64`, no floating point enters the log schema — and
gains three orders of magnitude of resolution. This is a field rename, not a unit change behind
the same name: a consumer (dashboard, alert) filtering or graphing the old `duration_ms` field
must be updated to `duration_us` and must not assume the two are interchangeable (`duration_us`
values are ~1000× larger than the old `duration_ms` values for the same duration).

---
