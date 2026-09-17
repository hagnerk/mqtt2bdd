# Core Workflows

Key system workflows illustrated using sequence diagrams with standardized actor naming and accurate behavior.

## Actor Naming Convention

- **Main** - Main Goroutine (lifecycle coordinator)
- **ConfigMgr** - Configuration Manager
- **Logger** - Structured Logger (slog)
- **MQTTClient** - MQTT Client component (includes Paho goroutine)
- **MsgChannel** - Message Channel (buffered, capacity 1000)
- **DBWriter** - Database Writer Goroutine
- **DBClient** - Database Client (pgx pool wrapper)
- **PostgreSQL** - PostgreSQL database
- **MQTTBroker** - MQTT Broker (Mosquitto)
- **Device** - Zigbee2MQTT device (message source)

## Workflow 1: Normal Operation - Message Capture Flow

```mermaid
sequenceDiagram
    participant Device as Zigbee Device
    participant MQTTBroker as MQTT Broker
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel<br/>(Buffer: 1000)
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL

    Note over Device,PostgreSQL: Normal Operation - Single Message Flow

    Device->>MQTTBroker: PUBLISH zigbee2mqtt/bedroom/temp<br/>{"temperature": 21.5}
    MQTTBroker->>MQTTClient: MESSAGE (QoS 0)

    MQTTClient->>MQTTClient: Extract topic, payload<br/>timestamp = time.Now()
    MQTTClient->>Logger: DEBUG: "Message received"
    MQTTClient->>MsgChannel: send Message{topic, ts, payload}
    Note over MsgChannel: Buffered (async)<br/>Count: 1/1000

    MsgChannel->>DBWriter: receive Message
    DBWriter->>DBClient: InsertMessage(sensor, date, metrics)
    DBClient->>PostgreSQL: INSERT INTO sensor_metrics<br/>VALUES ($1, $2, $3)
    PostgreSQL-->>DBClient: Success (1 row)
    DBClient-->>DBWriter: nil (no error)
    DBWriter->>Logger: DEBUG: "Message persisted"<br/>sensor=bedroom/temp

    Note over MQTTClient,DBWriter: Process continues for next message
```

## Workflow 2: Application Startup Sequence

```mermaid
sequenceDiagram
    participant Main as Main Goroutine
    participant ConfigMgr as Config Manager
    participant Logger as Logger
    participant MQTTClient as MQTT Client
    participant DBClient as Database Client
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant MQTTBroker as MQTT Broker
    participant PostgreSQL as PostgreSQL

    Note over Main,PostgreSQL: Application Startup Sequence

    Main->>ConfigMgr: LoadConfig()
    ConfigMgr->>ConfigMgr: Read env variables
    ConfigMgr-->>Main: Config{} or error
    alt Config error
        Main->>Main: Log FATAL + Exit(1)
    end

    Main->>Logger: InitLogger(config.LogLevel)
    Logger-->>Main: *slog.Logger
    Main->>Logger: INFO: "MQTT2BDD starting"<br/>version=X.Y.Z

    Main->>MQTTClient: NewClient(config, logger)
    MQTTClient-->>Main: *Client
    Main->>MQTTClient: Connect(ctx)
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT
    MQTTBroker-->>MQTTClient: CONNACK
    MQTTClient-->>Main: nil (success)
    Main->>Logger: INFO: "MQTT connected"<br/>broker=mosquitto:1883

    Main->>DBClient: NewClient(config, logger)
    DBClient-->>Main: *Client
    Main->>DBClient: Connect(ctx)
    DBClient->>PostgreSQL: Create connection pool<br/>(min: 1, max: 5)
    PostgreSQL-->>DBClient: Pool ready
    DBClient-->>Main: nil (success)
    Main->>Logger: INFO: "Database connected"<br/>host=postgres

    Main->>MsgChannel: make(chan Message, 1000)
    Main->>DBWriter: go runDBWriter(msgChan)
    activate DBWriter
    Note over DBWriter: for msg := range msgChan<br/>(blocked waiting)

    Main->>MQTTClient: Subscribe("#", messageHandler)
    MQTTClient->>MQTTBroker: SUBSCRIBE #
    MQTTBroker-->>MQTTClient: SUBACK
    MQTTClient-->>Main: nil (success)
    Main->>Logger: INFO: "Subscribed to all topics"

    Main->>Main: Block on signal channel<br/>sigChan := make(chan os.Signal)

    Note over Main,DBWriter: Application running - processing messages
```

## Workflow 3: Database Outage & Recovery

```mermaid
sequenceDiagram
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel<br/>(1000 capacity)
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL
    participant Logger as Logger

    Note over MQTTClient,Logger: Database Outage Scenario

    MQTTClient->>MsgChannel: send Message #1
    MsgChannel->>DBWriter: receive Message #1
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL--xDBClient: Connection refused (error)
    DBClient-->>DBWriter: error: connection failed
    DBWriter->>Logger: ERROR: "Database write failed"<br/>error=connection refused

    Note over PostgreSQL: Database offline

    MQTTClient->>MsgChannel: send Message #2
    MQTTClient->>MsgChannel: send Message #3
    MQTTClient->>MsgChannel: send Message #4
    Note over MsgChannel: Messages buffering<br/>Count: 4/1000 (Message #1 being retried)

    DBWriter->>DBWriter: time.Sleep(10 * time.Second)
    DBWriter->>Logger: INFO: "Retrying database write"<br/>attempt=1
    DBWriter->>DBClient: InsertMessage() [retry Message #1]
    DBClient->>PostgreSQL: INSERT
    PostgreSQL--xDBClient: Connection refused
    DBClient-->>DBWriter: error: still down

    MQTTClient->>MsgChannel: send Message #5
    MQTTClient->>MsgChannel: send Message #6
    Note over MsgChannel: Messages buffering<br/>Count: 6/1000

    DBWriter->>DBWriter: time.Sleep(10 * time.Second)
    DBWriter->>Logger: INFO: "Retrying database write"<br/>attempt=2
    DBWriter->>DBClient: InsertMessage() [retry Message #1]

    Note over PostgreSQL: Database back online

    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success (1 row)
    DBClient-->>DBWriter: nil (success)
    DBWriter->>Logger: INFO: "Database reconnected"
    DBWriter->>Logger: DEBUG: "Message persisted (retried)"

    MsgChannel->>DBWriter: receive Message #2
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #3
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    Note over DBWriter,PostgreSQL: Continue draining buffer<br/>until channel empty
```

## Workflow 4: Graceful Shutdown

```mermaid
sequenceDiagram
    participant OS as OS Signal<br/>(SIGTERM/SIGINT)
    participant Main as Main Goroutine
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL
    participant Logger as Logger

    Note over OS,Logger: Graceful Shutdown - docker stop

    OS->>Main: SIGTERM received<br/>(signal channel)
    Main->>Logger: INFO: "Shutdown signal received"

    Main->>MQTTClient: Disconnect()
    MQTTClient->>MQTTClient: Unsubscribe from topics<br/>Close broker connection
    MQTTClient-->>Main: Done
    Main->>Logger: INFO: "MQTT disconnected"

    Main->>MsgChannel: close(msgChan)
    Note over MsgChannel: Closed - no more sends allowed<br/>Existing messages preserved<br/>Count: 3 messages remaining

    Note over DBWriter: for msg := range msgChan<br/>(draining buffer)

    MsgChannel->>DBWriter: receive Message #1
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #2
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #3
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    Note over MsgChannel: Channel empty & closed

    DBWriter->>DBWriter: Exit for loop<br/>(channel closed & drained)
    DBWriter->>Main: Signal completion<br/>(via goroutine return)

    Main->>Main: Wait for DBWriter to finish<br/>(blocking via WaitGroup)
    Main->>Logger: INFO: "Flushed 3 messages"
    Main->>DBClient: Close()
    DBClient->>PostgreSQL: Close connection pool
    PostgreSQL-->>DBClient: Closed
    Main->>Logger: INFO: "Shutdown complete"
    Main->>Main: os.Exit(0)
```

## Workflow 5: MQTT Broker Disconnection & Reconnection

```mermaid
sequenceDiagram
    participant MQTTBroker as MQTT Broker
    participant MQTTClient as MQTT Client
    participant Logger as Logger
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client

    Note over MQTTBroker,DBClient: MQTT Broker Outage

    MQTTBroker--xMQTTClient: Connection lost<br/>(network failure)
    MQTTClient->>MQTTClient: onConnectionLost callback<br/>(configured in Paho)
    MQTTClient->>Logger: ERROR: "MQTT connection lost"<br/>reason=network error

    Note over MQTTClient: Paho auto-reconnect active<br/>(10 second interval configured)

    Note over MsgChannel,DBClient: Database writer continues<br/>processing existing buffer

    MsgChannel->>DBWriter: receive buffered Message
    DBWriter->>DBClient: InsertMessage()
    DBClient->>DBClient: Continue normal processing<br/>(MQTT outage doesn't block DB)

    MQTTClient->>MQTTClient: time.Sleep(10 * time.Second)
    MQTTClient->>Logger: INFO: "Attempting MQTT reconnection"<br/>attempt=1
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT
    MQTTBroker--xMQTTClient: Connection refused
    MQTTClient->>Logger: WARN: "Reconnection failed"

    MQTTClient->>MQTTClient: time.Sleep(10 * time.Second)
    MQTTClient->>Logger: INFO: "Attempting MQTT reconnection"<br/>attempt=2
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT

    Note over MQTTBroker: Broker back online

    MQTTBroker-->>MQTTClient: CONNACK
    MQTTClient->>Logger: INFO: "MQTT reconnected"

    MQTTClient->>MQTTBroker: SUBSCRIBE # (wildcard)
    MQTTBroker-->>MQTTClient: SUBACK
    MQTTClient->>Logger: INFO: "Resubscribed to all topics"

    Note over MQTTClient,MsgChannel: Resume message reception
    MQTTBroker->>MQTTClient: New messages
    MQTTClient->>MsgChannel: send Messages
```

## Buffering Limits & Message Loss Scenarios

**Critical Understanding: Multiple Buffer Layers**

```
Zigbee Device → MQTT Broker → [Paho Internal Buffer] → [App Message Channel] → Database
                                 (configurable)           (1000 capacity)
```

**Buffer Configuration:**
```go
opts := mqtt.NewClientOptions()
opts.SetMessageChannelDepth(5000)  // Paho internal buffer (default: 100)

msgChan := make(chan Message, 1000)  // Application channel
```

**Message Loss Risk Matrix:**

| Scenario | Duration | Buffering Capacity | Message Loss? |
|----------|----------|-------------------|---------------|
| DB outage | < 5 min | App channel sufficient | **No** |
| DB outage | 5-10 min | App + Paho buffers | **No** (if rate < 10 msg/sec) |
| DB outage | > 10 min | Both buffers overflow | **Yes** |
| MQTT outage | Any | N/A - source unavailable | **Yes** (messages never received) |
| App crash (SIGKILL) | Instant | Buffers lost | **Yes** (in-memory only) |
| Graceful shutdown | < 30 sec | Buffers flushed | **No** |

## Error Handling Summary

| Error Scenario | Behavior | Recovery | Data Loss? |
|---------------|----------|----------|------------|
| **Startup failure** (config/connection) | Log FATAL, Exit(1) | Manual intervention required | N/A (app never started) |
| **Database outage** (< buffer capacity) | Buffer messages in channel, retry every 10s | Automatic when DB recovers | **No** |
| **Database outage** (> buffer capacity) | Both buffers full, Paho drops new messages | Automatic reconnect, but messages lost | **Yes** (overflow messages) |
| **MQTT outage** | Auto-reconnect every 10s, resubscribe on connect | Automatic when broker recovers | **Yes** (messages during outage never received) |
| **Channel full** (DB very slow) | MQTT handler blocks, Paho buffers internally | Unblocks when DB writer drains channel | **Depends** (no if Paho buffer sufficient, yes if Paho also fills) |
| **Duplicate message** | UNIQUE constraint violation on (sensor, date) | Log WARNING, skip (idempotent via ON CONFLICT) | **No** (duplicate rejected) |
| **Payload with repairable content** (`\u0000`, unpaired surrogate escape, invalid UTF-8) | Repaired to U+FFFD, WARN `payload_sanitized` | Immediate | **No** (content visibly altered) |
| **Payload the database can never accept** (invalid JSON, SQLSTATE class 22/23/54) | Discarded, ERROR `write_rejected`, never retried | Immediate; the next message proceeds | **Yes** (that message only) |
| **Empty payload** (clears a retained message) | Skipped, DEBUG `write_skipped` | Immediate | **No** (nothing to store) |
| **Excluded topic** (`MQTT_EXCLUDE_TOPICS`) | Dropped before buffering, DEBUG `message_excluded` | N/A | **No** (excluded by configuration) |
| **Graceful shutdown** (SIGTERM) | Drain both buffers before exit (< 30s) | Clean shutdown with flush | **No** |
| **Forced kill** (SIGKILL) | Immediate termination, no cleanup | None | **Yes** (all buffered messages lost) |

**Key Takeaway:**

The system provides **best-effort durability** with ~5-10 minutes of buffer capacity during database outages (depending on message rate). This is sufficient for typical transient failures (restarts, network blips) but **not guaranteed storage**. For critical applications requiring zero message loss, additional persistence layers (disk-based queues, MQTT QoS 1/2) would be needed—trade-offs consciously avoided for this learning-focused project.

---
