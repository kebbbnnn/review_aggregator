# Movie Review Aggregator

**User:** Anonymous  
**Created:** 8/8/2026 20:25:35  
**Updated:** 8/8/2026 20:34:47  
**Exported:** 8/9/2026 10:30:32  
**Link:** [https://chatgpt.com/c/6a77203b-43e8-83ec-9681-2605a8c23622](https://chatgpt.com/c/6a77203b-43e8-83ec-9681-2605a8c23622)  

## Prompt:
8/8/2026, 8:34:22 PM

I want to build a program that runs in a server collecting latest movies, then scraping reddit, yts.bz, rotten tomato and imdb for reviews of each movie then use an LLM to write a summary for the movie. Is this possible using free resources like render.com free tier and firebase free tier?

## Response:
8/8/2026, 8:34:24 PM

> **Searching the web**
>
> [Deploy for Free – Render Docs](https://render.com/docs/free)
> [Rotten Tomatoes: Terms of Use | Rotten Tomatoes](https://www.rottentomatoes.com/policies/terms-of-use)
> [IMDb | Help](https://help.imdb.com/article/imdb/general-information/can-i-use-imdb-data-in-my-software/G5JTRESSHJBBHTGX)
> [Free LLM APIs (April 2026 Update)](https://www.reddit.com/r/clawdbot/comments/1sph0iu/free_llm_apis_april_2026_update/)
>
> **Searched 4 websites**
>
> 
>
> Worked for a couple of seconds

Yes - **technically, you can build this almost entirely with free-tier infrastructure**, especially as a hobby/personal project. But there are two separate questions:

1. **Can the architecture run for free?** → Mostly yes.
2. **Can you legally/reliably scrape all those sites?** → This is the much bigger problem.

Your proposed architecture is feasible, but I would change a few things.

### A practical architecture

```text
                    ┌─────────────────────┐
                    │   Scheduled Job     │
                    │   Render Cron       │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ Find latest movies │
                    │ TMDB / OMDb / etc.  │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │ Movie Queue / DB    │
                    │ Firebase Firestore  │
                    └──────────┬──────────┘
                               │
             ┌─────────────────┼──────────────────┐
             ▼                 ▼                  ▼
        Reddit/API        Movie data APIs      Other sources
             │                 │                  │
             └─────────────────┼──────────────────┘
                               ▼
                    ┌─────────────────────┐
                    │ Normalize reviews   │
                    │ + deduplicate       │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │       LLM           │
                    │ Summarize sentiment │
                    │ + reviews            │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ Firestore           │
                    │ Movie + summary     │
                    └─────────────────────┘
```

Render is actually a good fit for the scheduled part. Render supports cron jobs specifically for periodic tasks, although **cron jobs themselves aren't free** according to the current documentation; they have a minimum monthly charge. Free Render instances are available for web services, Postgres, and Key Value, but not every service type. ([Render](https://render.com/docs/free?utm_source=chatgpt.com))

So I would **not design around a Render Cron Job if "strictly $0" is a requirement.**

Instead, you could have:

```text
GitHub Actions
     │
     │ scheduled every 6 hours
     ▼
Render Web Service
     │
     ├── scrape/fetch movies
     ├── process reviews
     └── call LLM
              │
              ▼
          Firestore
```

GitHub Actions can act as the scheduler and call an endpoint on your Render service.

---

## The biggest issue: scraping

This is where I'd challenge your original plan.

You mentioned:

- Reddit
- YTS
- Rotten Tomatoes
- IMDb

I wouldn't build the system around **scraping all four websites**.

### IMDb

IMDb explicitly says that data mining, robots, screen scraping, and similar automated extraction from the website aren't allowed. They do provide datasets for certain non-commercial uses. ([IMDb Help](https://help.imdb.com/article/imdb/general-information/can-i-use-imdb-data-in-my-software/G5JTRESSHJBBHTGX?utm_source=chatgpt.com))

So rather than:

```text
your scraper → imdb.com → scrape reviews
```

use:

```text
IMDb datasets / permitted APIs
```

where appropriate.

### Rotten Tomatoes

This is even more problematic.

Their current Terms explicitly prohibit automated collection/data mining and specifically prohibit using automated methods such as robots, scripts, spiders and scrapers without authorization. Their terms also restrict using their content for developing/improving AI systems. ([Rotten Tomatoes](https://www.rottentomatoes.com/policies/terms-of-use?utm_source=chatgpt.com))

So **I would not scrape Rotten Tomatoes reviews and feed them into your LLM.**

### Reddit

Reddit is much more suitable if you use the **official API** rather than scraping Reddit's HTML.

For example:

```text
Search Reddit:
"Movie Title"
"Movie Title" review
"Movie Title" ending
"Movie Title" worth watching
```

Then collect relevant posts/comments and summarize the overall sentiment.

This also gives you something potentially more interesting than professional critic scores: **actual audience reactions**.

### YTS

I'd also separate YTS from the review system.

If your goal is simply:

> "Find newly released movies"

you don't really need YTS.

You could use a movie metadata source to identify releases, then use Reddit and other permitted sources for audience sentiment.

---

# I would actually build it like this

![Image](https://images.openai.com/static-rsc-4/4B9uqP9-PVQohwNNzHkY4oMZCv2RCr0QgDadLfwHMce5BWfIgaAU348BJ0qa8mfiGpSQtvqr1oeGHrdukQ30j3VGnczfJMsXRSiKU9q9GU7XkLVmIiOwipMkN2kkekWL2pE3aKHvDRysmpvlxb8tXQd21mlQmnWilc_SU6U3JSz9kR0A-uDIrUGwFEy9LAud?purpose=fullsize)

![Image](https://images.openai.com/static-rsc-4/YcLZoi73Xf7x0vhKmJPy6n6OE3jKbIbcuuz7zqXlMVWmKuCQugdpIYoFxXojjF5zVMFo2gLTB_E2iQ4Pt4vSejN83SxgkLABkankFSWnR5X_IlrezV2okAtGHwmEY49gY6y7VkTfHWO_ZWUm-ToarKqqW7ohGrcdS4GeuqmrAsxqQBmMoH3ffT5HGDn1lS8F?purpose=fullsize)

![Image](https://images.openai.com/static-rsc-4/uvCBEyFRuX9GxD03wBriDFFUtHW5yhHPeFo329kX3weUKJjJs1jk4ufI35PKE_ARkgs8XDLQ9GZ5cUJ9E8AwL_kRuauPAF3E2qJSqT1B7F-RYjOuZiV1dS_jq7cGrSR2iOTWvLn-PQXI-lP_AucxZccDMoUMBpxq8nT93VOTwPhPei0fjnTmms2bCCb5RHkW?purpose=fullsize)

![Image](https://images.openai.com/static-rsc-4/Y6Q0XKrdXW5AF6pCDPiHdO4V6GZIAKOsJGPn6MI3pWK1ZeTmU98VFFU_Flx8B205O_mTre7_cr3QSN8iivQJxSGaXKqv6dPUicngE73rZMPh-CjseJeiPnL_gnX4i-GMNROcVTH6clDo3eTp1S56grtjOgPrC4rqVGwV3Sy7jrckypw2J9esMQLxWzKOYKxr?purpose=fullsize)

![Image](https://images.openai.com/static-rsc-4/XrBuPJUJXOA49gezRsEyza_SZJ3STaeMeZdDCKj2FxMTQbmQuDNwvZ9JN1LUFcKag5ZD20OOxBKNRKI4Jo4evLbdeaEjAFLD7VqA8X4SML0qtYFGOphW9nCEwRhj-6UfKKCOOhjMnaZgJ7MtMEb33F7hkpLtdr43JqPD7xhzJEgo4RtigZhQIZfBK9J9gWEP?purpose=fullsize)

### 1. Movie discovery

Use a legitimate movie metadata API.

For example:

```text
TMDB
  ↓
Movies released recently
  ↓
Movie ID
Title
Release date
Poster
Genres
Overview
```

This gives you a canonical movie ID.

---

### 2. Review collection

For each movie:

```text
Movie: Superman

       ┌── Reddit
       │
       ├── IMDb permitted data
       │
       ├── other permitted review APIs
       │
       └── critic/review APIs
                ↓
          normalized reviews
```

I'd store something like:

```json
{
  "movieId": "tmdb_12345",
  "source": "reddit",
  "author": "...",
  "text": "...",
  "score": 152,
  "createdAt": "...",
  "url": "..."
}
```

Don't send everything to the LLM.

---

# 3. Preprocess before the LLM

This is extremely important if you're trying to stay within free LLM quotas.

Suppose you collect:

```text
Reddit:       500 comments
IMDb:         200 reviews
Other sites:  100 reviews
```

Don't do:

```text
600,000 tokens → LLM
```

Instead:

```text
                Reviews
                   │
                   ▼
            Remove duplicates
                   │
                   ▼
            Remove junk/spam
                   │
                   ▼
          Sentiment extraction
                   │
                   ▼
        Representative reviews
                   │
                   ▼
                  LLM
```

You can even do a two-stage process:

```text
1000 reviews
     │
     ▼
cheap/local processing
     │
     ▼
50 representative reviews
     │
     ▼
LLM
     │
     ▼
final summary
```

---

# 4. LLM

This part can potentially be free.

There are currently various providers offering free API quotas, although limits and availability change frequently. For example, current community reports list free quotas for Gemini, Cohere, Mistral and several inference providers. ([Reddit](https://www.reddit.com/r/clawdbot/comments/1sph0iu/free_llm_apis_april_2026_update/?utm_source=chatgpt.com))

I wouldn't architect your application around one provider, though.

Create an abstraction:

```typescript
interface LLMProvider {
    summarizeMovie(input: MovieReviewData): Promise<MovieSummary>
}
```

Then:

```text
             LLM interface
                  │
       ┌──────────┼──────────┐
       ▼          ▼          ▼
    Gemini      Groq       Mistral
```

If one free quota disappears, switch providers.

---

# 5. Firebase

Firebase is actually a reasonable choice for the database.

I'd use Firestore roughly like:

```text
movies/
    movieId/
        title
        releaseDate
        poster
        overview
        imdbRating
        rottenTomatoesScore
        redditSentiment
        summary
        lastUpdated

        reviews/
            reviewId/
                source
                text
                author
                score
                createdAt
```

But there's an important optimization:

**Don't store every Reddit comment forever.**

Instead, you could keep:

```text
Movie
 ├── metadata
 ├── scores
 ├── sentiment statistics
 ├── representative reviews
 └── generated summary
```

That dramatically reduces Firestore storage and reads.

---

# The really interesting part

I wouldn't make the LLM simply generate:

> "This movie is good because..."

You could make the system produce a much richer analysis.

For example:

```json
{
  "overallSentiment": 78,
  "audienceSentiment": 82,
  "criticSentiment": 74,

  "pros": [
    "Visual effects",
    "Lead performance",
    "Action sequences"
  ],

  "cons": [
    "Pacing",
    "Predictable plot"
  ],

  "commonThemes": [
    "nostalgia",
    "action",
    "character development"
  ],

  "audienceConsensus":
    "Most viewers enjoyed the action and performances,
     while criticism focused on pacing.",

  "recommendation":
    "Worth watching if you enjoy superhero action films."
}
```

Then your application becomes less like a movie database and more like an **AI movie-review aggregator**.

---

# Can it actually be $0?

For a **small personal project**, potentially yes.

| Component | Free possibility |
|---|---|
| Backend | Render free Web Service |
| Scheduler | GitHub Actions |
| Database | Firebase/Firestore free quota |
| Movie metadata | Free API tier |
| Reddit | Official API/free access subject to limits |
| LLM | Provider free tier |
| Frontend | Firebase Hosting / Render static site |
| Images | Don't download/rehost; store URLs |
| Domain | ❌ Usually costs money |

The main constraint won't necessarily be compute.

It will be **API quotas and data-source restrictions**.

Render's free web services also spin down after 15 minutes without inbound traffic and have an ephemeral filesystem, so don't treat the local filesystem as your database. ([Render](https://render.com/docs/free?utm_source=chatgpt.com))

---

## One architecture I'd recommend for you

Given that you're trying to keep the project free:

```text
                 GitHub Actions
                 Scheduled trigger
                       │
                       ▼
                Render Web API
                       │
          ┌────────────┴────────────┐
          ▼                         ▼
    Movie Metadata               Reddit API
          │                         │
          └────────────┬────────────┘
                       ▼
                  Firestore
                       │
                       ▼
               Review Processor
                       │
                       ▼
                  LLM Provider
                       │
                       ▼
                Movie Summary
                       │
                       ▼
                  Firestore
                       │
                       ▼
                 Your Frontend
```

And I would **remove direct IMDb/Rotten Tomatoes scraping from the design** unless you obtain permission/licensed access. That's not merely a technical obstacle; their current terms make automated extraction problematic. ([Rotten Tomatoes](https://www.rottentomatoes.com/policies/terms-of-use?utm_source=chatgpt.com))

### One more important distinction

If this is just:

> **personal/hobby project**

you have considerably more flexibility in how you experiment, subject to each service's terms.

If you're planning:

> **public website / monetized SaaS**

then I'd redesign the data layer around **licensed APIs and sources that explicitly permit reuse**, because building a business on scraped IMDb/RT content is a significant risk.

If you want, I can also design the **complete $0 architecture**, including the exact APIs, Firestore schema, GitHub Actions schedule, Render deployment, scraping/collection pipeline, and LLM prompt structure.

---

**Sources:**

- [Deploy for Free – Render Docs](https://render.com/docs/free?utm_source=chatgpt.com)
- [IMDb | Help](https://help.imdb.com/article/imdb/general-information/can-i-use-imdb-data-in-my-software/G5JTRESSHJBBHTGX?utm_source=chatgpt.com)
- [Rotten Tomatoes: Terms of Use | Rotten Tomatoes](https://www.rottentomatoes.com/policies/terms-of-use?utm_source=chatgpt.com)
- [Free LLM APIs (April 2026 Update)](https://www.reddit.com/r/clawdbot/comments/1sph0iu/free_llm_apis_april_2026_update/?utm_source=chatgpt.com)



---
Powered by [ChatGPT Exporter](https://www.chatgptexporter.com)