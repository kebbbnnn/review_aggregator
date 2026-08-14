# 🎬 Movie Review Aggregator

An automated, $0-budget backend pipeline and API for movie review aggregation. The pipeline runs as a **Go CLI** on scheduled **GitHub Actions**, discovering recently released movies, gathering audience reviews across Reddit and Letterboxd, pre-processing text, generating structured sentiment summaries with an OpenAI-compatible LLM, and persisting data to **Cloudflare D1**.

A lightweight **Cloudflare Worker** serves consumer-facing read APIs globally at the edge with sub-millisecond database queries.

---

## 🌟 Key Features

* **Continuous Full-Catalog Movie Discovery**: Crawls TMDB's full movie database (~900,000+ movies) with a dedicated stateful catalog pipeline that resumes across GitHub Actions runs using artifact cursors.
* **Smart Deep Processing**: Selectively triggers review collection and LLM summarization for popular recent releases (popularity ≥ 50, released in last 6 months).
* **Multi-Source Review Collection**: Concurrently gathers audience posts and reviews using Reddit's OAuth2 API (`r/movies`) and Letterboxd public RSS feeds (`golang.org/x/sync/errgroup`).
* **Smart Preprocessing**: Cleans URLs, filters out short/junk comments, deduplicates repetitive text blocks, and ranks top reviews to optimize LLM token usage.
* **Flexible LLM Provider Support**: Generic HTTP client using the standard OpenAI `/v1/chat/completions` API schema — zero code changes to switch between Google Gemini, Groq, OpenRouter, or local models.
* **Normalized Cloudflare D1 Storage**: Persists movies, genres, pros/cons, and themes in a relational SQLite schema on Cloudflare D1 (5GB free tier).
* **Edge API via Cloudflare Worker**: Fast, low-latency search, single-movie lookup, and genre/score filtering served via Cloudflare Workers.
* **Strict $0 Infrastructure**: No always-on servers required. Eliminates Render compute by running the Go pipeline directly on GitHub Actions.

---

## 🏗️ Architecture

```text
       ┌──────────────────────────────┐          ┌──────────────────────────────┐
       │     GitHub Actions: Sync     │          │    GitHub Actions: Catalog   │
       │       (Cron: Every 6h)       │          │       (Cron: Every 2h)       │
       └──────────────┬───────────────┘          └──────────────┬───────────────┘
                      │ runs go run ./cmd/pipeline              │ runs go run ./cmd/catalog
                      ▼                                         ▼
       ┌──────────────────────────────┐          ┌──────────────────────────────┐
       │   Deep Review Pipeline CLI   │          │     TMDB Catalog Crawler     │
       │   (Popular Recent Releases)  │          │    (Full Catalog Pagination) │
       └──────────────┬───────────────┘          └──────────────┬───────────────┘
                      │                                         │
        ┌─────────────┼─────────────┐                           │
        ▼             ▼             ▼                           │
 ┌──────────────┐┌──────────────┐┌──────────────┐               │
 │   TMDB API   ││   OMDb API   ││  Reddit API  │               │
 │ (Popular Rec)││  (Ratings)   ││ (Audience)   │               │
 └──────────────┘└──────────────┘└──────────────┘               │
                                        │                       │
                                        ▼                       │
                                 ┌──────────────┐               │
                                 │ Letterboxd   │               │
                                 │ (RSS Feed)   │               │
                                 └──────────────┘               │
                      │                                         │
                      ▼                                         │
       ┌──────────────────────────────┐                         │
       │   Review Processor / Clean   │                         │
       └──────────────┬───────────────┘                         │
                      │                                         │
                      ▼                                         │
       ┌──────────────────────────────┐                         │
       │   LLM Client (OpenAI-compat) │                         │
       │   (Structured Summary)       │                         │
       └──────────────┬───────────────┘                         │
                      │ writes deep summary                     │ writes metadata
                      └────────────────────┬────────────────────┘
                                           ▼
                               ┌───────────────────────┐
                               │  Cloudflare D1 (DB)   │
                               │ (Normalized Schema)   │
                               └───────────▲───────────┘
                                           │ native D1 binding
                               ┌───────────┴───────────┐
                               │   Cloudflare Worker   │
                               │     (Edge API)        │
                               └───────────▲───────────┘
                                           │ HTTPS requests
                               ┌───────────┴───────────┐
                               │  Web / Mobile Clients │
                               └───────────────────────┘
```

---

## 📂 Project Structure

```text
.
├── cmd/
│   ├── catalog/         # Go CLI for full TMDB catalog discovery & backfill
│   │   └── main.go
│   └── pipeline/        # Go CLI entrypoint for deep review & LLM sync
│       └── main.go
│
├── internal/
│   ├── catalog/         # Catalog orchestrator, rate limiting, and cursor management
│   ├── config/          # Environment variables & .env loader
│   ├── discovery/       # TMDB & OMDb API integrations
│   ├── collector/       # Review collectors (Reddit OAuth2 API, Letterboxd RSS)
│   ├── processor/       # Text deduplication, cleaning, and ranking
│   ├── llm/             # Generic OpenAI-compatible chat completion client
│   ├── store/           # Cloudflare D1 REST client & data models
│   └── pipeline/        # Orchestrator tying discovery -> collection -> LLM -> storage
│
├── worker/              # Cloudflare Worker (TypeScript API Gateway)
│   ├── src/
│   │   ├── index.ts     # Request router & CORS handling
│   │   ├── db.ts        # D1 query helpers (search, getById, list/filter)
│   │   └── types.ts     # TypeScript interfaces
│   ├── schema.sql       # D1 relational database schema
│   ├── wrangler.toml    # Cloudflare Wrangler configuration
│   ├── package.json
│   └── tsconfig.json
│
├── .github/
│   └── workflows/
│       ├── catalog_sync.yml    # Runs TMDB catalog crawler every 2h on GitHub Actions
│       ├── scheduled_sync.yml  # Runs deep review pipeline every 6h on GitHub Actions
│       └── deploy_worker.yml   # Deploys Worker on push to main
│
├── .env.example         # Template environment variable configuration
├── go.mod
└── go.sum
```

---

## ⚙️ Environment Variables (Go Pipeline)

Copy `.env.example` to `.env` and fill in your API credentials:

```bash
cp .env.example .env
```

| Environment Variable | Required | Description | Default |
|---|---|---|---|
| `TMDB_API_KEY` | **Yes** | TMDB API v3 Key for discovering releases | — |
| `OMDB_API_KEY` | Optional | OMDb API Key for IMDb & Rotten Tomatoes scores | — |
| `REDDIT_CLIENT_ID` | Optional | Reddit OAuth App Client ID | — |
| `REDDIT_CLIENT_SECRET` | Optional | Reddit OAuth App Client Secret | — |
| `REDDIT_USER_AGENT` | No | User agent header for Reddit requests | `MovieReviewAggregator/1.0` |
| `LLM_BASE_URL` | No | Base URL for OpenAI-compatible LLM endpoint | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `LLM_API_KEY` | Optional | API key for LLM provider (needed for deep summary) | — |
| `LLM_MODEL` | No | LLM model identifier | `gemini-2.5-flash` |
| `CF_ACCOUNT_ID` | **Yes** | Cloudflare Account ID | — |
| `CF_D1_DATABASE_ID` | **Yes** | Cloudflare D1 Database UUID | — |
| `CF_API_TOKEN` | **Yes** | Cloudflare API Token with D1 Edit permissions | — |
| `MAX_MOVIES_PER_SYNC` | No | Max movies to process in deep pipeline per run | `10` |
| `MIN_POPULARITY` | No | Minimum TMDB popularity for deep review pipeline | `50.0` |
| `RECENT_MONTHS` | No | How many months back to consider for deep review pipeline | `6` |

---

## 🚀 Getting Started

### Prerequisites
* **Go 1.24+**
* **Node.js 20+ & npm**
* [Cloudflare Account](https://dash.cloudflare.com/) (Free Tier)
* API Keys:
  * [TMDB API Key](https://www.themoviedb.org/settings/api) (Free)
  * [OMDb API Key](https://www.omdbapi.com/apikey.aspx) (Free 1,000 req/day)
  * [Reddit API App](https://www.reddit.com/prefs/apps) (Free script app)
  * Gemini API Key (or Groq / OpenRouter)

---

### Database Setup (Cloudflare D1)

1. **Install Wrangler & Login**:
   ```bash
   cd worker
   npx wrangler login
   ```

2. **Create D1 Database**:
   ```bash
   npx wrangler d1 create movie-review-aggregator
   ```
   Note the `database_id` output and update `worker/wrangler.toml`.

3. **Apply Schema**:
   ```bash
   # Local development database (.wrangler/)
   npx wrangler d1 execute movie-review-aggregator --local --file=schema.sql --yes

   # Remote Cloudflare D1 database
   npx wrangler d1 execute movie-review-aggregator --remote --file=schema.sql --yes
   ```

---

### Running the Go Pipeline Locally

1. **Run unit tests**:
   ```bash
   go test -v ./...
   ```

2. **Execute catalog crawler (full discovery & metadata backfill)**:
   ```bash
   go run ./cmd/catalog --cursor=cursor.json --max-pages=5
   ```

3. **Execute deep review & LLM pipeline manually**:
   ```bash
   go run ./cmd/pipeline
   ```

---

### Running the Cloudflare Worker Locally

```bash
cd worker
npm install
npm run dev
```

Visit:
* `http://localhost:8787/healthz`
* `http://localhost:8787/api/v1/movies`
* `http://localhost:8787/api/v1/movies/search?q=superman`
* `http://localhost:8787/api/v1/movies/tmdb_12345`

---

## 🌐 API Endpoints (Cloudflare Worker)

| Method | Endpoint | Description | Query Parameters |
|---|---|---|---|
| `GET` | `/healthz` | Health check endpoint | — |
| `GET` | `/api/v1/movies/search` | Search movies by title | `q` (required), `limit` (default: 10, max: 50) |
| `GET` | `/api/v1/movies/:id` | Get full movie summary by ID | — |
| `GET` | `/api/v1/movies` | Browse & filter movies | `genre`, `sort` (`release_date`, `imdb_score`, `rotten_tomatoes`, `overall_sentiment`, `last_updated`), `order` (`asc`/`desc`), `limit`, `offset` |

---

## ☁️ GitHub Actions & Deployment Guide

### Step 1: Configure Repository Secrets
Navigate to your GitHub repository and go to **Settings > Secrets and variables > Actions > New repository secret**.

Add the following secrets:

| Secret Name | Description | Required? |
|---|---|---|
| `CF_ACCOUNT_ID` | Your Cloudflare Account ID | **Yes** |
| `CF_D1_DATABASE_ID` | Your Cloudflare D1 Database UUID | **Yes** |
| `CF_API_TOKEN` | Cloudflare API Token (`D1:Edit`, `Workers:Edit`) | **Yes** |
| `TMDB_API_KEY` | TMDB API v3 Key | **Yes** |
| `LLM_API_KEY` | OpenAI/Gemini/Groq API Key | Optional (for deep summaries) |
| `OMDB_API_KEY` | OMDb API Key | Optional |
| `LLM_BASE_URL` | Custom OpenAI-compatible endpoint URL | Optional |
| `LLM_MODEL` | Custom model identifier | Optional |
| `REDDIT_CLIENT_ID` | Reddit App Client ID | Optional |
| `REDDIT_CLIENT_SECRET` | Reddit App Secret | Optional |
| `MAX_MOVIES_PER_SYNC` | Limit per sync run (default: 10) | Optional |
| `MIN_POPULARITY` | Min popularity for deep sync (default: 50.0) | Optional |
| `RECENT_MONTHS` | Months back for deep sync (default: 6) | Optional |

---

### Step 2: Workflows

The repository includes two automated workflows:

#### 1. Catalog Movie Sync (`.github/workflows/catalog_sync.yml`)
- **Schedule**: Every 2 hours (`cron: '30 */2 * * *'`)
- **Action**: Crawls TMDB's full movie catalog, saves metadata + genres to D1 in batches, and persists its progress across runs via GitHub Actions artifact cursors.

#### 2. Scheduled Movie Review Sync (`.github/workflows/scheduled_sync.yml`)
- **Schedule**: Every 6 hours (`cron: '0 */6 * * *'`)
- **Action**: Discovers popular recent movies, gathers Reddit + Letterboxd reviews, generates LLM summaries, and writes detailed sentiment scores and pros/cons to D1.

#### Manual Trigger (On Demand)
1. Go to the **Actions** tab on your GitHub repository.
2. Select either **"Catalog Movie Sync"** or **"Scheduled Movie Review Sync"**.
3. Click the **Run workflow** dropdown on the right and select `Branch: main`.

---

### Step 3: Viewing Real-Time Execution Logs
1. Go to the **Actions** tab.
2. Click on the latest workflow run.
3. Click on the **Run Go Pipeline** job.
4. You will see streaming stdout/stderr output detailing movie discovery, review collection, LLM processing, and Cloudflare D1 writes.

---

### Step 4: Worker Continuous Deployment
Any push to `main` that modifies files inside `worker/**` will automatically trigger [`.github/workflows/deploy_worker.yml`](.github/workflows/deploy_worker.yml) to deploy the Cloudflare Worker.

To deploy manually from your machine:
```bash
cd worker
npx wrangler deploy
```

---

## 📝 Example JSON Response (`/api/v1/movies/:id`)

```json
{
  "result": {
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
}
```

---

## 🎨 CineScope Web Frontend

A modern, dark cinematic web interface for exploring movies and reading AI-generated audience reviews.

### Features
* **Discover Grid**: Browse 9,600+ cataloged movies with sorting by AI sentiment score, release date, IMDb score, or Rotten Tomatoes rating.
* **Instant Genre Filters**: Filter across Action, Animation, Sci-Fi, Horror, Comedy, and more.
* **Full AI Intelligence Reports**: Rich detail view displaying the audience consensus quote, "Who Should Watch This" recommendations, pros/cons breakdown, and common discussion themes.
* **Real-time Search**: Search across thousands of titles with instant results.
* **Zero Backend Costs**: Built as a pure static site (HTML5/CSS3/Vanilla JS) connecting to Cloudflare Worker API.

### Running Locally
```bash
cd website
python3 -m http.server 8080
# Open http://localhost:8080 in your browser
```

### Deploying to Render.com
1. Connect this repository to your [Render.com](https://render.com) dashboard.
2. Render automatically detects [`render.yaml`](render.yaml) as a **Static Site**:
   - **Publish Directory**: `./website`
   - **Build Command**: `echo "No build required"`
3. Click **Deploy**.

---

## 📜 License

MIT License.
