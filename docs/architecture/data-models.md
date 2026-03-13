# Data Models

Based on the PRD requirements, MQTT2BDD has a minimal but well-designed data model focused on flexible sensor data capture.

## Model: Message (In-Memory)

**Purpose:** Internal representation of MQTT messages flowing through the application pipeline (channel-based communication between goroutines)

**Key Attributes:**
- **Topic** (`string`) - MQTT topic path identifying the sensor/device (e.g., "zigbee2mqtt/living_room/thermostat")
- **Timestamp** (`time.Time`) - Message receipt timestamp in UTC, used for temporal ordering and analysis
- **Payload** (`[]byte`) - Raw MQTT message payload as byte slice, preserved for flexible JSON storage

**Relationships:**
- No direct relationships - this is a data transfer object (DTO) used internally
- Maps 1:1 to `sensor_metrics` table rows upon persistence

**Design Rationale:**
- **`[]byte` payload:** Preserves raw MQTT data without premature parsing, allows PostgreSQL JSONB to handle schema-less sensor data
- **UTC timestamps:** Ensures consistent temporal analysis across time zones
- **Minimal structure:** Only captures essential fields, avoids over-engineering for this pipeline use case

## Model: sensor_metrics (PostgreSQL Table)

**Purpose:** Persistent storage of all MQTT sensor messages for historical analysis, trending, and Grafana visualization

**Schema Definition:**

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| **id** | BIGINT | GENERATED ALWAYS AS IDENTITY, PRIMARY KEY | Auto-incrementing unique identifier, managed exclusively by PostgreSQL |
| **sensor** | VARCHAR(255) | NOT NULL | MQTT topic path serving as sensor identifier (e.g., "zigbee2mqtt/bedroom/temperature") |
| **date** | TIMESTAMP | NOT NULL | Message timestamp in UTC for time-series analysis |
| **metrics** | JSONB | NOT NULL | Raw JSON payload containing sensor readings (temperature, humidity, state, etc.) |
| | | UNIQUE (sensor, date) | Composite constraint preventing duplicate messages |

**Key Design Decisions:**

1. **IDENTITY vs SERIAL:**
   - Uses modern **SQL:2003 standard** `GENERATED ALWAYS AS IDENTITY` instead of legacy `BIGSERIAL`
   - `ALWAYS` mode prevents manual ID insertion (strict protection against developer errors)
   - Sequence intrinsically tied to column (cleaner lifecycle management)
   - Better portability if database migration needed in future

2. **BIGINT for ID:**
   - Range: -9,223,372,036,854,775,808 to +9,223,372,036,854,775,807
   - At 1 million inserts/day = 25,000+ years before overflow
   - Minimal storage overhead (8 bytes per row)
   - Future-proof for extreme scaling scenarios

3. **UNIQUE(sensor, date) Constraint:**
   - Prevents duplicate message insertion (same sensor + timestamp)
   - TIMESTAMP precision to microsecond makes collisions extremely rare
   - Provides data integrity without application-level deduplication logic
   - INSERT fails with constraint violation if duplicate detected

4. **JSONB (not JSON) for metrics:**
   - Supports efficient indexing on specific JSON keys (e.g., `metrics->>'temperature'`)
   - Binary storage format (faster than text-based JSON)
   - Enables Grafana queries like: `SELECT metrics->>'temperature' FROM sensor_metrics WHERE sensor = 'bedroom/temp'`
   - _Trade-off:_ Slightly larger storage vs JSON, but query performance gain is substantial

**Benefits of ID Column for Future Extensibility:**
- Simple foreign keys: Future tables (alerts, annotations) reference via single `BIGINT` column
- Efficient pagination: Cursor-based with `WHERE id > last_id LIMIT 100`
- Performant JOINs: Integer comparison faster than composite key joins
- Standard tooling: ORMs, admin tools expect single-column PKs

## Data Flow Mapping

```
MQTT Message                    Message Struct                  sensor_metrics Row
-------------                   --------------                  ------------------
topic: "zigbee/bedroom/temp"    Topic: "zigbee/bedroom/temp"   id: 42 (auto-generated)
payload: {"temp": 21.5}    -->  Payload: [bytes]          -->  sensor: "zigbee/bedroom/temp"
timestamp: (broker time)        Timestamp: time.Now()          date: 2026-02-13 14:30:00.123456
                                                               metrics: {"temp": 21.5}
```

**Design Philosophy:**

The data model embraces **schema-on-read** rather than schema-on-write. MQTT devices publish heterogeneous JSON structures (thermostats send `{temperature, humidity}`, switches send `{state}`, energy meters send `{power, voltage, current}`). Storing raw JSONB allows the application to be device-agnostic while enabling Grafana to extract relevant fields at query time.

The addition of an `id` column provides a stable, efficient primary key for future extensibility (alerting, annotations, audit logs) while the `UNIQUE(sensor, date)` constraint maintains data integrity for the time-series use case.

---
