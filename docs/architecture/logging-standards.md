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
| `main` | `starting`, `unknown_log_level`, `config_load_failed`, `config_loaded`, `buffer_initialised`, `writer_started`, `health_monitor_started`, `startup_complete`, `startup_aborted`, `buffer_full`, `buffer_high`, `message_dropped`, `health_check`, `shutdown_signal`, `flush_complete`, `flush_timeout`, `shutdown_complete` |
| `mqtt` | `connecting`, `connected`, `connect_timeout`, `connect_failed`, `connection_lost`, `reconnecting`, `reconnect_failed`, `reconnected`, `reconnect_cancelled`, `disconnected`, `subscribing`, `subscribed`, `subscribe_failed`, `resubscribe_failed`, `message_received` |
| `database` | `connecting`, `connected`, `connect_failed`, `disconnected`, `write_success`, `write_failure`, `write_duplicate`, `topic_truncated`, `pool_stats` |

`unknown_log_level` is emitted from the `logger` package itself but tagged `component=main`, because
it reports on the application's own configuration rather than on the logger as a subsystem.

## `operation` Vocabulary

`operation` is **required on every ERROR or WARN entry that reports a failed attempt**, and names
what was being attempted: `load_config`, `connect`, `reconnect`, `subscribe`, `parse_dsn`, `ping`,
`insert`.

Entries that report a *state* rather than a failed attempt carry **no** `operation`, whatever their
level — there is no operation to name. These are `connection_lost` at ERROR, and `buffer_full`,
`buffer_high`, `message_dropped`, `flush_timeout`, `write_duplicate`, `topic_truncated`,
`unknown_log_level` at WARN.

`connection_lost` is the single ERROR in that list, and the reason the rule is not phrased "every
ERROR carries an operation": the broker dropped the link on its own, no local call was attempted, so
any `operation` value would be fabricated.

## Domain Fields

| Area | Fields |
|------|--------|
| Message processing (`write_success` / `write_failure` / `write_duplicate`) | `topic`, `payload_size` (bytes), `duration_ms` |
| Health check (`health_check`) | `mqtt_status`, `db_status`, `db_last_write_age_s` (`-1` ⇒ no successful write since startup), `buffer_used`, `buffer_capacity`, `buffer_utilization_percent`, `processed_last_interval` |
| Buffer pressure (`buffer_high`) | `buffer_used`, `buffer_capacity`, `buffer_utilization_percent` |
| Startup / shutdown | `version`, `log_level`, `effective_log_level`, `buffer_size`, `buffer_capacity`, `interval_s`, `signal`, `count`, `buffered`, `failed_component` |

`failed_component` is deliberately not `component`: an entry naming a culprit is still emitted by
`main`, and `component` must keep identifying the emitter.

`log_level` is the value the operator requested; `effective_log_level` is the level actually in
force after an unrecognised value has resolved to INFO. Both appear on `config_loaded`, so a
mismatch is diagnosable from that single entry.

---
