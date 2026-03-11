# ARGUS

**AI-powered real-time safety inspection.**
Speak to it. Point a camera at it. It tells you what's dangerous.

ARGUS is a multimodal Live Agent built on the Gemini Live API. It streams audio and video in real time, reasons about hazards using loaded inspection rule modules, highlights them with AR overlays, and speaks findings back to the user — all with sub-second latency and full interruption support.

---

## Hackathon Category — Live Agents

**Mandatory tech:** Gemini Live API (bidirectional audio + video streaming)
**Hosted on:** Google Cloud Run

---

## What It Does

| Capability | How |
|---|---|
| Real-time hazard detection | Video frames streamed to Gemini Live at 2 fps |
| Voice conversation | PCM audio streamed bidirectionally; Gemini responds in audio |
| Interruption handling | `ActivityHandlingStartOfActivityInterrupts` — user can cut off mid-response |
| Wake word | Say **"argus"** to start/stop inspection hands-free |
| AR overlays | Hazards highlighted with severity-coloured rings over the live camera feed |
| Inspection modules | Swappable rule sets: construction, warehouse, electrical, facility |
| Report generation | JSON / PDF inspection reports with full hazard log |
| Adaptive UI | Three interfaces auto-detected: Smartphone · CCTV dashboard · AR headset |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         FRONTEND                                │
│                    Next.js 14 · TypeScript                      │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │  Smartphone  │  │     CCTV     │  │    AR / Headset      │  │
│  │   Session    │  │   Session    │  │      Session         │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘  │
│         └─────────────────┴──────────────────────┘             │
│                            │                                    │
│                   WebSocket (wss://)                            │
│         JPEG frames · PCM audio · Control events               │
└────────────────────────────┬────────────────────────────────────┘
                             │
                ┌────────────▼───────────┐
                │   Google Cloud Run     │
                │   Go 1.24 Backend      │
                │                       │
                │  WebSocket Server      │
                │       │               │
                │  Agent Controller     │
                │  ┌────┴────────────┐  │
                │  │  Vision Pipeline│  │
                │  │  Frame Sampler  │  │
                │  │  Event Engine   │  │
                │  └────────────────┘  │
                │  ┌─────────────────┐  │
                │  │  Rule Engine    │  │
                │  │  Modules: 4     │  │
                │  └─────────────────┘  │
                │  ┌─────────────────┐  │
                │  │  Report Builder │  │
                │  │  JSON · PDF     │  │
                │  └─────────────────┘  │
                └────────────┬──────────┘
                             │
                 ┌───────────▼────────────┐
                 │  Google Gemini Live    │
                 │  API (GenAI SDK)       │
                 │                       │
                 │  gemini-2.5-flash-     │
                 │  native-audio-preview  │
                 │                       │
                 │  ← audio stream        │
                 │  ← video frames        │
                 │  → audio response      │
                 │  → text + tool calls   │
                 └───────────────────────┘

GCP Services:
  Cloud Run          — hosts the Go backend
  Cloud Build        — builds container from source (cloudbuild.yaml)
  Artifact Registry  — stores Docker images
  Secret Manager     — stores GEMINI_API_KEY at rest
```

---

## Tech Stack

**Backend**
- Go 1.24
- `google.golang.org/genai` — official Google GenAI SDK
- Gemini Live API — `gemini-2.5-flash-native-audio-preview`
- Gemini GenerateContent API — `gemini-2.5-flash` (frame analysis fallback)
- Gorilla WebSocket

**Frontend**
- Next.js 14 · TypeScript · Tailwind CSS
- Web Speech API (wake word + voice commands)
- Web Audio API (PCM audio streaming)
- WebXR (AR context detection)

**Google Cloud**
- Cloud Run (hosting)
- Cloud Build (IaC — `cloudbuild.yaml`)
- Artifact Registry (container images)
- Secret Manager (API key)

---

## Local Development

### Prerequisites

- Go 1.24+
- Node.js 18+
- A [Gemini API key](https://aistudio.google.com/app/apikey)

### 1. Clone & configure

```bash
git clone https://github.com/cutmob/argus
cd argus
cp .env.example .env
# Edit .env and set GEMINI_API_KEY=your_key_here
```

### 2. Run the backend

```bash
make run
# Server starts on :8080
# Health check: http://localhost:8080/api/v1/health
```

### 3. Run the frontend

```bash
make frontend-install
make frontend-dev
# Open http://localhost:3001
```

---

## Cloud Deployment

### Prerequisites

- [gcloud CLI](https://cloud.google.com/sdk/docs/install) installed and authenticated
- A GCP project with billing enabled

### Deploy (one command)

```bash
make deploy PROJECT=your-gcp-project-id
```

This script automatically:
1. Enables Cloud Run, Cloud Build, Artifact Registry, and Secret Manager APIs
2. Creates the Artifact Registry repository
3. Stores your Gemini API key in Secret Manager
4. Builds the Go container via Cloud Build (no Docker required locally)
5. Deploys to Cloud Run with WebSocket session affinity and 1-hour timeout
6. Outputs the live service URL

After deployment, set the WebSocket URL in your frontend:
```bash
NEXT_PUBLIC_WS_URL=wss://your-service-url.run.app/ws
```

### CI/CD via Cloud Build

To automate deployments on every push to `main`:

```bash
# One-off manual trigger
make deploy-cloudbuild PROJECT=your-gcp-project-id

# Or connect your GitHub repo to a Cloud Build trigger in GCP Console
# — it will use cloudbuild.yaml automatically
```

---

## Inspection Modules

Modules are loaded from `./modules/`. Each module is a directory containing:

| File | Purpose |
|---|---|
| `metadata.json` | Name, version, author, tags |
| `rules.json` | Array of hazard detection rules with severity |
| `prompt.txt` | System prompt injected into the Gemini Live session |

**Built-in modules:** `construction` · `elevator` · `facility`

Add a new module:
```bash
make new-module NAME=warehouse
# Edit modules/warehouse/rules.json and modules/warehouse/prompt.txt
```

---

## API Reference

| Endpoint | Description |
|---|---|
| `WS /ws` | WebSocket — frames, audio, control events |
| `GET /api/v1/health` | Health check |
| `GET /api/v1/sessions` | List active sessions |
| `GET /api/v1/sessions/:id` | Get session details |
| `GET /api/v1/modules` | List available inspection modules |
| `POST /api/v1/reports` | Generate inspection report |
| `GET /api/v1/reports/:id` | Retrieve a report |

### WebSocket message types (client → server)

```json
{ "type": "frame",  "data": "<base64 JPEG>" }
{ "type": "audio",  "data": "<base64 PCM 16kHz mono>" }
{ "type": "event",  "event": "start_inspection", "mode": "construction" }
{ "type": "event",  "event": "stop_inspection" }
{ "type": "event",  "event": "generate_report" }
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `GEMINI_API_KEY` | Yes | Google Gemini API key |
| `PORT` | No | HTTP port (default: `8080`) |
| `ARGUS_MODULES_DIR` | No | Path to modules (default: `./modules`) |
| `GEMINI_LIVE_MODEL` | No | Override Live model name |
| `GEMINI_CONTENT_MODEL` | No | Override content model name |

---

## License

AGPL-3.0 with commercial licensing — see [LICENSE](LICENSE).
