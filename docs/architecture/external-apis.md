# External APIs

MQTT2BDD is a self-contained bridge application that does not integrate with external REST APIs, SaaS platforms, or third-party web services. The application only connects to infrastructure components:

1. **MQTT Broker (Mosquitto)** - MQTT protocol connection, not HTTP/REST API (covered in Components section)
2. **PostgreSQL Database** - Database protocol connection, not HTTP/REST API (covered in Components section)
3. **Grafana** - Downstream consumer that queries PostgreSQL directly; MQTT2BDD does not call Grafana APIs

**Decision:** No external API integrations required for this project.

---
