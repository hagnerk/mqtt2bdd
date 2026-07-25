# mqtt2bdd — Project Instructions

## Language: English everywhere in this repository

**This project overrides the global language preferences.** In `~/.claude/CLAUDE.md`,
French is allowed for documentation aimed at the maintainer or clients (README, docs).
**That exemption does not apply here.**

Everything committed to this repository is written in English, without exception:

- source code — identifiers, comments, log messages, error strings;
- configuration files and their comments (`.gitignore`, `Dockerfile`, YAML, JSON…);
- commit messages;
- **all documentation** — `README.md`, `docs/` (PRD, architecture, stories, QA gates),
  and this file.

**Why:** mqtt2bdd is a public, portfolio-facing Go project. A reader who does not speak
French must be able to pick up any artefact in the repository — not just the code — and
understand it. A bilingual repository is worse than either language chosen consistently.

**French remains the language of direct conversation with the maintainer** (chat only).
Nothing written in French ever lands in a file.

**Known exceptions (legacy, do not propagate):** `docs/stories/1.1`, `1.7` and `1.9`
contain French passages in their Dev Notes and Dev Agent Record sections. They predate
this rule. Leave them as historical record; do not use them as a precedent when drafting
or reviewing new stories.
