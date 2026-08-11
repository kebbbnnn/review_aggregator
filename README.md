# 🎬 Movie Review Aggregator

An automated, $0-budget backend pipeline written in **Go** that discovers recently released movies, collects audience reviews across multiple sources (Reddit & Letterboxd), pre-processes text, generates structured sentiment summaries using an OpenAI-compatible LLM, and persists data to **Firebase Firestore**.

Designed for serverless execution on **Render (Free Tier)** triggered by a periodic **GitHub Actions** cron job.

---

## 🌟 Key Features

* **Movie Discovery & Rating Enrichment**: Discovers recent releases via **TMDB API** and enriches scores with **OMDb API** (IMDb ratings & Rotten Tomatoes scores).
* **Multi-Source Review Collection**: Concurrently gathers audience posts and reviews using Reddit's OAuth2 API (`r/movies`) and Letterboxd public RSS feeds (`golang.org/x/sync/errgroup`).
* **Smart Preprocessing**: Cleans URLs, filters out short/junk comments, deduplicates repetitive text blocks, and ranks top reviews to optimize LLM token usage.
* **Flexible LLM Provider Support**: Generic HTTP client using the standard OpenAI `/v1/chat/completions` API schema — zero code changes to switch between Google Gemini, Groq, OpenRouter, or local models.
* **Firestore Persistence**: Stores structured summaries in Firestore with a 24-hour freshness cache to avoid redundant API calls.
* **Strict $0 Infrastructure**: Built specifically to fit within free tiers of Render, GitHub Actions, TMDB, OMDb, and Firebase.

---

## 🏗️ Architecture

```text
               ┌───────────────────────┐
               │    GitHub Actions     │
               │   (Cron: Every 6h)    │
               └───────────┬───────────┘
                           │ POST /api/v1/jobs/sync
                           ▼
               ┌───────────────────────┐
               │   Render Web Service  │
               │      (Go Server)      │
               └───────────┬───────────┘
                           │
       ┌───────────────────┼───────────────────┐
       ▼                   ▼                   ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   TMDB API   │    │   OMDb API   │    │  Reddit API  │
│ (Discovery)  │    │  (Ratings)   │    │ (Audience)   │
└──────────────┘    └──────────────┘    └──────────────┘
                                               │
                                               ▼
                                        ┌──────────────┐
                                        │ Letterboxd   │
                                        │ (RSS Feed)   │
                                        └──────────────┘
                           │
                           ▼
               ┌───────────────────────┐
               │   Review Processor    │
               │ (Clean / Filter / Cap)│
               └───────────┬───────────┘
                           │
                           ▼
               ┌───────────────────────┐
               │   LLM Client (OpenAI) │
               │ (Structured Summary)  │
               └───────────┬───────────┘
                           │
                           ▼
               ┌───────────────────────┐
               │   Firebase Firestore  │
               │     (Persistence)     │
               └───────────────────────┘
```

---

## 📂 Project Structure

```text
.
├── cmd/
│   └── server/          # Main entrypoint & HTTP server (/healthz, /api/v1/jobs/sync, /api/v1/movies/search)

├── internal/
│   ├── config/          # Environment variables & .env loader
│   ├── discovery/       # TMDB & OMDb API integrations
│   ├── collector/       # Review collectors (Reddit OAuth2 API, Letterboxd RSS)
│   ├── processor/       # Text deduplication, cleaning, and ranking
│   ├── llm/             # Generic OpenAI-compatible chat completion client
│   ├── store/           # Firebase Firestore persistence layer
│   └── pipeline/        # Orchestrator tying discovery -> collection -> LLM -> storage
├── .github/
│   └── workflows/       # Scheduled GitHub Action workflow
├── .env.example         # Template environment variable configuration
├── Dockerfile           # Multi-stage Docker container for Render deployment
├── go.mod
└── go.sum
```

---

## ⚙️ Environment Variables

Copy `.env.example` to `.env` and fill in your API credentials:

```bash
cp .env.example .env
```

| Environment Variable | Required | Description | Default |
|---|---|---|---|
| `PORT` | No | HTTP server port | `8080` |
| `CRON_SECRET` | **Yes** | Shared secret header (`X-Cron-Secret`) for securing `/api/v1/jobs/sync` | — |
| `TMDB_API_KEY` | **Yes** | TMDB API v3 Key for discovering releases | — |
| `OMDB_API_KEY` | Optional | OMDb API Key for IMDb & Rotten Tomatoes scores | — |
| `REDDIT_CLIENT_ID` | Optional | Reddit OAuth App Client ID | — |
| `REDDIT_CLIENT_SECRET` | Optional | Reddit OAuth App Client Secret | — |
| `REDDIT_USER_AGENT` | No | User agent header for Reddit requests | `MovieReviewAggregator/1.0` |
| `LLM_BASE_URL` | No | Base URL for OpenAI-compatible LLM endpoint | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `LLM_API_KEY` | **Yes** | API key for LLM provider | — |
| `LLM_MODEL` | No | LLM model identifier | `gemini-2.5-flash` |
| `FIREBASE_PROJECT_ID` | Optional | GCP Firebase Project ID for Firestore | — |
| `MAX_MOVIES_PER_SYNC` | No | Max movies to discover per run | `10` |

---

## 🚀 Getting Started

### Prerequisites
* **Go 1.24+**
* API Keys:
  * [TMDB API Key](https://www.themoviedb.org/settings/api) (Free)
  * [OMDb API Key](https://www.omdbapi.com/apikey.aspx) (Free 1,000 req/day)
  * [Reddit API App](https://www.reddit.com/prefs/apps) (Free script app)
  * Gemini API Key (or Groq / OpenRouter)

### Running Locally

1. **Clone the repository**:
   ```bash
   git clone https://github.com/your-username/review_aggregator.git
   cd review_aggregator
   ```

2. **Configure environment variables**:
   ```bash
   cp .env.example .env
   # Edit .env with your API keys
   ```

3. **Run unit tests**:
   ```bash
   go test -v ./...
   ```

4. **Start the Go server**:
   ```bash
   go run ./cmd/server
   ```

5. **Trigger manual pipeline sync**:
   ```bash
   curl -X POST http://localhost:8080/api/v1/jobs/sync \
     -H "X-Cron-Secret: super_secret_cron_token_123"
   ```

6. **Query movies by title**:
   ```bash
   curl "http://localhost:8080/api/v1/movies/search?q=inception"
   ```


---

## ☁️ Deployment Guide

### 1. Deploy to Render (Web Service)
1. Push your repository to GitHub.
2. Create a new **Web Service** on [Render](https://render.com/).
3. Choose **Docker** as the runtime. Render will automatically build using the included [`Dockerfile`](file:///Users/kebbbnnn/Projects/Studies/review_aggregator/Dockerfile).
4. Set the **Environment Variables** on Render (`CRON_SECRET`, `TMDB_API_KEY`, `LLM_API_KEY`, etc.).

### 2. Configure GitHub Actions Cron Schedule
Add two secrets under your GitHub repository settings (**Settings > Secrets and variables > Actions**):
* `RENDER_SERVICE_URL`: `https://your-app-name.onrender.com`
* `CRON_SECRET`: Must match the `CRON_SECRET` configured on Render.

The workflow in [`.github/workflows/scheduled_sync.yml`](file:///Users/kebbbnnn/Projects/Studies/review_aggregator/.github/workflows/scheduled_sync.yml) will trigger every 6 hours automatically.

---

## 📝 Data Format (`movies/{tmdb_id}`)

Here is an example document saved to Firestore:

```json
{
  "id": "tmdb_12345",
  "tmdb_id": 12345,
  "imdb_id": "tt1234567",
  "title": "Superman",
  "release_date": "2025-07-11T00:00:00Z",
  "poster_url": "https://image.tmdb.org/t/p/w500/...",
  "overview": "A heroic journey...",
  "genres": ["Action", "Sci-Fi"],
  "scores": {
    "imdb": "7.8",
    "rotten_tomatoes": "84%"
  },
  "summary": {
    "overall_sentiment": 82,
    "pros": ["Visual effects", "Lead actor performance"],
    "cons": ["Third act pacing"],
    "common_themes": ["Nostalgia", "Character arc"],
    "audience_consensus": "Most viewers enjoyed the visuals, though pacing lagged slightly.",
    "recommendation": "Recommended for fans of comic book adaptations."
  },
  "review_count_analyzed": 24,
  "last_updated": "2026-08-09T10:00:00Z"
}
```

---

## 📜 License

MIT License.
