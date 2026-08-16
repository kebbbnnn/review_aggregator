# 🎬 Movie Review Aggregator

An automated, $0-budget backend pipeline and API for movie review aggregation. The pipeline runs as a **Go CLI** on scheduled **GitHub Actions**, discovering recently released movies, gathering audience reviews across Reddit and Letterboxd, pre-processing text, generating structured sentiment summaries with an OpenAI-compatible LLM, and persisting data to **Cloudflare D1**.

A lightweight **Cloudflare Worker** serves consumer-facing read APIs globally at the edge with sub-millisecond database queries.

---

## 🌟 Key Features

* **Continuous Full-Catalog Movie Discovery**: Crawls TMDB's full movie database (~900,000+ movies) with a dedicated stateful catalog pipeline that resumes across GitHub Actions runs using artifact cursors.
* **Smart Deep Processing**: Selectively triggers review collection and LLM summarization for popular recent releases (popularity ≥ 20, released in last 6 months).
* **Multi-Source Review Collection**: Concurrently gathers audience posts and reviews using Reddit's OAuth2 API (`r/movies`) and Letterboxd public RSS feeds (`golang.org/x/sync/errgroup`).
* **Smart Preprocessing**: Cleans URLs, filters out short/junk comments, deduplicates repetitive text blocks, and ranks top reviews to optimize LLM token usage.
* **Flexible LLM Provider Support**: Generic HTTP client using the standard OpenAI `/v1/chat/completions` API schema — zero code changes to switch between Google Gemini, Groq, OpenRouter, or local models.
* **Dual D1 Database Architecture**: Separates full TMDB catalog metadata (`movie-review-aggregator` on Account 1) from rich LLM audience review summaries (`movie-summaries` on Account 2), giving independent 5GB capacity pools (10GB total free tier storage).
* **Summary Worker with Edge Caching**: Dedicated read-only microservice for LLM summaries on Account 2, queried seamlessly and cached at the edge via Cloudflare Cache API (1-hour TTL).
* **Graceful Degradation**: If summary services are unreachable or undergoing maintenance, the consumer API continues serving rich catalog metadata without errors.
* **Migration Tooling**: Includes built-in migration utility (`cmd/migrate_summaries`) to sync and partition data between databases with zero downtime.

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
       └───────┬──────────────┬───────┘                         │
writes summary │              │ writes metadata                 │ writes metadata
               ▼              └────────────────┬────────────────┘
    ┌───────────────────────┐                  ▼
    │  Cloudflare D1 (DB2)  │       ┌───────────────────────┐
    │  (Account 2: Summary) │       │  Cloudflare D1 (DB1)  │
    └──────────▲────────────┘       │  (Account 1: Catalog) │
               │ native D1          └──────────▲────────────┘
    ┌──────────┴────────────┐                  │ native D1
    │     Summary Worker    │                  │ binding
    │  (Account 2: Edge API)│                  │
    └──────────▲────────────┘                  │
               │ HTTP + Cache                  │
               └───────────────┬───────────────┘
                               ▼
                    ┌─────────────────────┐
                    │    Primary Worker   │
                    │ (Account 1: API GW) │
                    └──────────▲──────────┘
                               │ HTTPS requests
                    ┌──────────┴──────────┐
                    │ CineScope Frontend  │
                    └─────────────────────┘
```

---

## 📂 Project Structure

```text
.
├── cmd/
│   ├── catalog/             # Go CLI for full TMDB catalog discovery & backfill (DB1)
│   │   └── main.go
│   ├── pipeline/            # Go CLI for deep review & LLM sync (DB1 + DB2)
│   │   └── main.go
│   └── migrate_summaries/   # One-time migration utility from DB1 to DB2
│       └── main.go
│
├── internal/
│   ├── catalog/             # Catalog orchestrator, rate limiting, and cursor management
│   ├── config/              # Environment variables & .env loader
│   ├── discovery/           # TMDB & OMDb API integrations
│   ├── collector/           # Review collectors (Reddit OAuth2 API, Letterboxd RSS)
│   ├── processor/           # Text deduplication, cleaning, and ranking
│   ├── llm/                 # Generic OpenAI-compatible chat completion client
│   ├── store/               # Cloudflare D1 REST client & dual-database storage models
│   └── pipeline/            # Orchestrator tying discovery -> collection -> LLM -> storage
│
├── worker/                  # Primary Cloudflare Worker (Account 1: API Gateway + Edge Cache)
│   ├── src/
│   │   ├── index.ts         # Request router & CORS handling
│   │   ├── db.ts            # DB1 metadata query helpers (search, getById, list)
│   │   ├── summary-client.ts# Edge-cached client for Summary Worker queries
│   │   └── types.ts         # TypeScript interfaces
│   ├── schema.sql           # DB1 schema (movies metadata + genres)
│   ├── wrangler.toml        # Account 1 Wrangler configuration
│   ├── package.json
│   └── tsconfig.json
│
├── worker-summaries/        # Summary Cloudflare Worker (Account 2: LLM Summaries Microservice)
│   ├── src/
│   │   ├── index.ts         # Summary router with API key authentication
│   │   ├── db.ts            # DB2 summary query helpers (single & batch)
│   │   └── types.ts         # TypeScript interfaces
│   ├── schema.sql           # DB2 schema (summaries, points, themes)
│   ├── wrangler.toml        # Account 2 Wrangler configuration
│   ├── package.json
│   └── tsconfig.json
│
├── .github/
│   └── workflows/
│       ├── catalog_sync.yml          # Runs TMDB catalog crawler every 2h on GitHub Actions
│       ├── scheduled_sync.yml        # Runs deep review pipeline every 6h on GitHub Actions
│       ├── deploy_worker.yml         # Deploys Primary Worker to Account 1 on push to main
│       └── deploy_summary_worker.yml # Deploys Summary Worker to Account 2 on push to main
│
├── .env.example             # Template environment variable configuration
├── go.mod
└── go.sum
```

---

## ⚙️ Environment Variables (Go Pipeline & Workers)

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
| `CF_ACCOUNT_ID` | **Yes** | Cloudflare Account ID (DB1 / Primary) | — |
| `CF_D1_DATABASE_ID` | **Yes** | Cloudflare D1 Database UUID (DB1) | — |
| `CF_API_TOKEN` | **Yes** | Cloudflare API Token with D1 Edit permissions | — |
| `CF_SUMMARY_ACCOUNT_ID` | Optional | Cloudflare Account ID (DB2: defaults to CF_ACCOUNT_ID) | — |
| `CF_SUMMARY_DATABASE_ID` | Optional | Cloudflare D1 Database UUID (DB2: defaults to CF_D1_DATABASE_ID) | — |
| `CF_SUMMARY_API_TOKEN` | Optional | Cloudflare API Token for DB2 (defaults to CF_API_TOKEN) | — |
| `SUMMARY_WORKER_URL` | Optional | URL of deployed Summary Worker (e.g., https://movie-summaries-api.workers.dev) | — |
| `SUMMARY_API_KEY` | Optional | Shared secret key between Primary Worker and Summary Worker | — |
| `MAX_MOVIES_PER_SYNC` | No | Max movies to process in deep pipeline per run | `10` |
| `MIN_POPULARITY` | No | Minimum TMDB popularity for deep review pipeline | `20.0` |
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

#### 1. Setup DB1: Metadata Database (Account 1)
```bash
cd worker
npx wrangler login
npx wrangler d1 create movie-review-aggregator
# Note database_id and update worker/wrangler.toml

# Apply Schema
npx wrangler d1 execute movie-review-aggregator --local --file=schema.sql --yes
npx wrangler d1 execute movie-review-aggregator --remote --file=schema.sql --yes
```

#### 2. Setup DB2: Summaries Database (Account 2)
```bash
cd ../worker-summaries
npx wrangler login
npx wrangler d1 create movie-summaries
# Note database_id and update worker-summaries/wrangler.toml

# Apply Schema
npx wrangler d1 execute movie-summaries --local --file=schema.sql --yes
npx wrangler d1 execute movie-summaries --remote --file=schema.sql --yes
```

---

### Migrating Existing Summaries (One-Time Utility)

If you have existing summary data in DB1 and want to copy it over to DB2:
```bash
go run ./cmd/migrate_summaries
```

---

### 🔍 Querying & Managing Dual Databases via CLI (Wrangler)

The database schema is partitioned across two separate Cloudflare accounts:

| Database | Account | Location | Tables |
|---|---|---|---|
| **DB1** (`movie-review-aggregator`) | **Account 1** | `worker/` | `movies`, `movie_genres` |
| **DB2** (`movie-summaries`) | **Account 2** | `worker-summaries/` | `movie_summaries`, `movie_points`, `movie_themes` |

#### 1. Querying DB1 (Account 1: Metadata & Catalog)
If your Wrangler session is logged in with Account 1:
```bash
cd worker

# Count total catalog movies in DB1
npx wrangler d1 execute movie-review-aggregator --remote --command="SELECT COUNT(*) as total_movies FROM movies;"

# Search recent movies in DB1
npx wrangler d1 execute movie-review-aggregator --remote --command="SELECT id, title, release_date FROM movies ORDER BY release_date DESC LIMIT 5;"
```

#### 2. Querying DB2 (Account 2: LLM Summaries, Points & Themes)
Because Wrangler CLI defaults to your active browser login, query Account 2 without logging out by passing Account 2's token and account ID via environment variables:

```bash
cd worker-summaries

# Count total movie points (pros & cons) in DB2
CLOUDFLARE_API_TOKEN="<CF_SUMMARY_API_TOKEN>" CLOUDFLARE_ACCOUNT_ID="<CF_SUMMARY_ACCOUNT_ID>" \
  npx wrangler d1 execute movie-summaries --remote --command="SELECT COUNT(*) as total_points FROM movie_points;"

# Breakdown of points by type (pro vs con) in DB2
CLOUDFLARE_API_TOKEN="<CF_SUMMARY_API_TOKEN>" CLOUDFLARE_ACCOUNT_ID="<CF_SUMMARY_ACCOUNT_ID>" \
  npx wrangler d1 execute movie-summaries --remote --command="SELECT type, COUNT(*) as count FROM movie_points GROUP BY type;"

# Count total summarized movies in DB2
CLOUDFLARE_API_TOKEN="<CF_SUMMARY_API_TOKEN>" CLOUDFLARE_ACCOUNT_ID="<CF_SUMMARY_ACCOUNT_ID>" \
  npx wrangler d1 execute movie-summaries --remote --command="SELECT COUNT(*) as total_summaries FROM movie_summaries;"

# Inspect sample summary in DB2
CLOUDFLARE_API_TOKEN="<CF_SUMMARY_API_TOKEN>" CLOUDFLARE_ACCOUNT_ID="<CF_SUMMARY_ACCOUNT_ID>" \
  npx wrangler d1 execute movie-summaries --remote --command="SELECT movie_id, overall_sentiment, audience_consensus FROM movie_summaries LIMIT 3;"
```


---

### Running the Go Pipeline Locally

1. **Run unit tests**:
   ```bash
   go test -v ./...
   ```

2. **Execute catalog crawler (full discovery & metadata backfill to DB1)**:
   ```bash
   go run ./cmd/catalog --cursor=cursor.json --max-pages=5
   ```

3. **Execute deep review & LLM pipeline (writes metadata to DB1, summaries to DB2)**:
   ```bash
   go run ./cmd/pipeline
   ```

---

### Running the Workers Locally

1. **Start Summary Worker (Account 2 microservice)**:
   ```bash
   cd worker-summaries
   npm install
   npm run dev -- --port 8788
   ```

2. **Start Primary Worker (Account 1 API Gateway)**:
   ```bash
   cd ../worker
   npm install
   SUMMARY_WORKER_URL="http://localhost:8788" npm run dev -- --port 8787
   ```

Visit:
* `http://localhost:8787/healthz`
* `http://localhost:8787/api/v1/movies`
* `http://localhost:8787/api/v1/movies/search?q=superman`
* `http://localhost:8787/api/v1/movies/tmdb_12345`


---

## 🌐 API Endpoints (Cloudflare Worker)

| Method | Endpoint | Description | Query Parameters / Body |
|---|---|---|---|
| `GET` | `/healthz` | Health check endpoint | — |
| `GET` | `/api/v1/movies/search` | Search movies by title | `q` (required), `limit` (default: 10, max: 50) |
| `GET` | `/api/v1/movies/:id` | Get full movie summary by ID | — |
| `GET` | `/api/v1/movies` | Browse & filter movies | `genre`, `sort` (`release_date`, `imdb_score`, `rotten_tomatoes`, `overall_sentiment`, `last_updated`), `order` (`asc`/`desc`), `limit`, `offset` |
| `POST` | `/api/v1/movies/:id/request-review` | Trigger on-demand AI review processing (6h cooldown per movie) | — |

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
 
The repository includes three automated workflows:
 
#### 1. Catalog Movie Sync (`.github/workflows/catalog_sync.yml`)
- **Schedule**: Every 2 hours (`cron: '30 */2 * * *'`)
- **Action**: Crawls TMDB's full movie catalog, saves metadata + genres to D1 in batches, and persists its progress across runs via GitHub Actions artifact cursors.
 
#### 2. Scheduled Movie Review Sync (`.github/workflows/scheduled_sync.yml`)
- **Schedule**: Every 6 hours (`cron: '0 */6 * * *'`)
- **Action**: Discovers popular recent movies (popularity ≥ 20), gathers Reddit + Letterboxd reviews, generates LLM summaries, and writes detailed sentiment scores and pros/cons to DB2.

#### 3. On-Demand Movie Review (`.github/workflows/on_demand_review.yml`)
- **Trigger**: Triggered via `workflow_dispatch` when a user clicks **"Generate AI Review"** on the website for any cataloged movie.
- **Action**: Runs `go run ./cmd/pipeline --movie-id=<id>`, prioritizing the requested movie first, then batching up to N other unreviewed movies.

#### Configuring On-Demand Reviews for the Primary Worker:
To enable the Primary Worker to trigger on-demand GitHub Actions workflows:
1. Generate a GitHub Personal Access Token (PAT) with `repo` or `actions:write` scope.
2. Set the secret on the Primary Worker:
   ```bash
   cd worker
   npx wrangler secret put GITHUB_TOKEN
   # Paste your GitHub PAT when prompted
   ```

#### Manual Trigger (On Demand via GitHub UI)
1. Go to the **Actions** tab on your GitHub repository.
2. Select any workflow (**"Catalog Movie Sync"**, **"Scheduled Movie Review Sync"**, or **"On-Demand Movie Review"**).
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
