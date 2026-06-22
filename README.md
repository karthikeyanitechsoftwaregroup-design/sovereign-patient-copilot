# Sovereign — Patient Journey Copilot

A local-first clinical dashboard where doctors upload patient PDFs and receive AI-powered next best actions — processed entirely on their own machine with zero data leaving their infrastructure.

---

## Quick Start

```bash
# 1. Clone and enter the repo
git clone https://github.com/your-username/sovereign
cd sovereign

# 2. Set your Anthropic API key
cp .env.example .env
# Edit .env and paste your key

# 3. Boot everything
docker compose up --build

# 4. Open the app
open http://localhost:3000
```

That's it. No external cloud services. No patient data leaves your machine.

---

## How to Run (Development)

### Backend (Go)
```bash
cd backend
go mod tidy
export ANTHROPIC_API_KEY=sk-ant-...
export DATABASE_URL=postgres://sovereign:sovereign@localhost:5432/sovereign?sslmode=disable
go run ./cmd/server
# → http://localhost:8080
```

### Frontend (React + Vite)
```bash
cd frontend
npm install
npm run dev
# → http://localhost:3000
```

---

## Architecture

```
Doctor's Browser
      │
      │  HTTP / SSE
      ▼
┌─────────────┐
│  React UI   │  Vite · Nginx (prod)
└──────┬──────┘
       │ /api/*
       ▼
┌─────────────┐
│  Go Backend │  Gin · Port 8080
│  (Gin)      │
└──┬──────┬───┘
   │      │
   │      ├──► Postgres  (patient_records, suggestions)
   │      │
   │      └──► Qdrant    (vector embeddings — future semantic search)
   │
   ├──► Agent A (Scribe)      ──► Anthropic API (claude-haiku-4-5)
   │
   └──► Agent B (Specialist)  ──► Anthropic API (claude-sonnet-4-6)
```

---

## How the Agents Work

### Agent A — Scribe (`claude-haiku-4-5`)

**Role:** Fast extraction and de-identification.

**Why Haiku?** This step is purely extractive — it needs precise instruction following but not deep reasoning. Haiku is 3× faster and significantly cheaper, making it ideal for the high-throughput de-identification step.

**What it does:**
1. Receives raw PDF text
2. Strips all PHI (name, DOB, MRN, address, phone) → replaces with `[REDACTED]`
3. Extracts vitals, diagnoses, medications, labs, ECG/imaging findings
4. Outputs a concise 4–6 sentence clinical summary

### Agent B — Specialist (`claude-sonnet-4-6`)

**Role:** Deep clinical reasoning and recommendation generation.

**Why Sonnet?** The specialist step requires nuanced multi-step reasoning: it must cross-reference findings, apply clinical guidelines, prioritise by urgency, and write precise rationale with citations. Sonnet's larger context and stronger reasoning make it the right choice here.

**What it does:**
1. Receives the de-identified summary from Agent A
2. Returns exactly 4 evidence-based "Next Best Actions" as structured JSON
3. Each action includes: title, description, priority (urgent/high/moderate), and a citation pointing back to the specific finding in the record

---

## How AI Routing Works

```go
func modelForRole(role string) string {
    switch role {
    case "scribe":
        return "claude-haiku-4-5-20251001"   // fast + cheap for extraction
    case "specialist":
        return "claude-sonnet-4-6"            // powerful for reasoning
    }
}
```

The routing is explicit and intentional:
- **Scribe → Haiku**: extraction tasks are well-defined and don't need frontier reasoning. Routing here saves cost and latency.
- **Specialist → Sonnet**: clinical recommendation requires broad medical knowledge, prioritisation logic, and structured output quality.

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/api/health` | Health check |
| `POST` | `/api/upload` | Upload PDF → SSE stream of agent pipeline |
| `GET`  | `/api/records` | List all stored records |
| `GET`  | `/api/records/:id` | Get record + suggestions |
| `POST` | `/api/emr/post/:id` | Post treatment plan to mock EMR |

### SSE Event Types (from `/api/upload`)

| Event | Payload | Meaning |
|-------|---------|---------|
| `pipeline_stage` | `{stage: 1-3, label: "..."}` | Progress update |
| `scribe_done` | `{summary: "..."}` | Agent A finished |
| `suggestions` | `[{title, description, priority, citation}]` | Agent B finished |
| `record_id` | `{id: 42}` | Saved to Postgres |
| `done` | `{stage: 4}` | Pipeline complete |
| `error` | `{message: "..."}` | Something went wrong |

---

## Database Schema

```sql
CREATE TABLE patient_records (
    id             BIGSERIAL PRIMARY KEY,
    filename       TEXT        NOT NULL,
    raw_text       TEXT        NOT NULL,     -- original PDF text
    scribe_summary TEXT,                     -- Agent A output (de-identified)
    created_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE suggestions (
    id          BIGSERIAL PRIMARY KEY,
    record_id   BIGINT REFERENCES patient_records(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    priority    TEXT NOT NULL,               -- urgent | high | moderate
    citation    TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Privacy Design

- PHI is stripped by Agent A **before** Agent B ever sees the data
- Raw PDF text is stored in Postgres but never sent to external services after Agent A processes it
- Everything runs locally via Docker — no patient data leaves your network
- Qdrant is included for future semantic search over de-identified summaries only

---

## Bonus: Mock EMR Integration

`POST /api/emr/post/:id` simulates posting a treatment plan to an EMR system (Epic FHIR sandbox). In production, replace the mock response with a real MCP tool call or FHIR R4 `Encounter` + `CarePlan` resource creation.

---

## Project Structure

```
sovereign/
├── docker-compose.yml
├── .env.example
├── backend/
│   ├── cmd/server/main.go          # entrypoint
│   ├── internal/
│   │   ├── agents/agents.go        # Agent A + B + AI routing
│   │   ├── db/db.go                # Postgres connect + migrate
│   │   ├── handlers/handlers.go    # Gin routes + SSE upload
│   │   └── models/models.go        # shared types
│   ├── pkg/pdf/extract.go          # PDF text extraction
│   ├── go.mod
│   └── Dockerfile
└── frontend/
    ├── src/
    │   ├── App.jsx                 # main UI + SSE client
    │   ├── App.css
    │   ├── main.jsx
    │   └── index.css
    ├── index.html
    ├── vite.config.js
    ├── nginx.conf
    ├── package.json
    └── Dockerfile
```
