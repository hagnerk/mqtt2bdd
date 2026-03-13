# Introduction

This document outlines the overall project architecture for MQTT2BDD, including backend systems, shared services, and non-UI specific concerns. Its primary goal is to serve as the guiding architectural blueprint for AI-driven development, ensuring consistency and adherence to chosen patterns and technologies.

**Relationship to Frontend Architecture:**
MQTT2BDD is a backend-only service with no user interface components. This document serves as the complete architectural specification.

## Starter Template or Existing Project

**Decision:** N/A - No starter template required for this Go project.

**Rationale:**
- Go projects don't typically use heavy scaffolding tools like Create React App or Vue CLI
- The PRD already specifies the exact directory structure (cmd/, internal/, pkg/)
- Starting from scratch provides better learning opportunities (one of the stated project goals)
- The dev environment is fully containerized, so no local Go installation complexity

## Change Log

| Date | Version | Description | Author |
|------|---------|-------------|--------|
| 2026-02-13 | 1.0 | Initial architecture document creation | Winston (Architect Agent) |

---
