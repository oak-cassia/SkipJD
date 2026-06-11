# skipJD

A Go-based job recommendation system that scrapes gamejob postings, structures them with an LLM, and matches them against user profiles.

> Built as a 5-week study project on **when an LLM belongs in a system and when it does not**.
> Study log: [hwichan-agent](https://github.com/MODAC-Agent/hwichan-agent) · [Final retrospective (Medium)](https://medium.com/@gnlckswjd1/llm%EC%9D%84-%EC%9E%98-%ED%99%9C%EC%9A%A9%ED%95%98%EB%A0%A4%EB%8B%A4-%EB%8B%A4%EC%8B%9C-%EB%A7%88%EC%A3%BC%EC%B9%9C-%EB%AC%B8%EC%A0%9C-%EC%A0%95%EC%9D%98%EC%9D%98-%EC%A4%91%EC%9A%94%EC%84%B1-750b721312e4)

## What It Does

skipJD targets one recruiting portal (gamejob) and runs the pipeline below.

1. **Crawler** collects postings; image-only bodies are flagged for later OCR.
2. **OCR worker** turns image bodies into text (via `gemini-cli`) and merges them with the HTML body.
3. **Extractor** turns each body into a structured `experience / competency / trait` JSON (via `gemini-cli`).
4. **Matcher** compares the structured posting against a user's preferences, normalized company ID, job codes, and years of experience.

```
[gamejob crawler]
  │  HTML body   → job_posting_bodies
  │    · no image     → ready_for_llm = true
  │    · has image    → ready_for_llm = false
  │  image URLs  → job_posting_images
  ▼
[OCR worker (LLM)]
  Targets rows with images but no OCR text yet.
  Writes "<html body> + [OCR] <ocr text>" back to the body row.
  → ready_for_llm = true
  ▼
job_posting_bodies   (only ready_for_llm = true rows proceed)
  ▼
[extractor (LLM)]
  body → experience / competency / trait JSON
  ▼
[structured matching input]
  + experience / competency / trait
  + job codes (multi-valued)
  + normalized company ID
  + user preferences · years of experience
  ▼
[recommendation matcher] → [user recommendations]
```

The three batch workers (crawler, ocr-worker, extractor) are gated by the `ready_for_llm` flag on each body row, so they do **not** depend on execution order or concurrency — running them in any order, or all at once, converges to the same result.

## Engineering Highlights

- **LLM only at the unstructured→structured boundary.** The system uses an LLM in exactly two places (extractor, OCR worker), both calling the same tool (`gemini-cli`). Matching, company-name normalization, and retry/timeout policies are all plain code.
- **Closed-set inputs over LLM normalization.** User-facing inputs (preferred company, job code) pick from crawler-curated lists rather than free text, which eliminates an entire LLM normalization pipeline that was designed end-to-end and then dropped.
- **Idempotent batch workers.** `ready_for_llm` flag plus `body.updated_at` checks let any worker be re-run at any time without producing duplicates or inconsistent state.
- **Uniform reliability layer.** Crawler · extractor · ocr-worker share a single `internal/retry` helper — HTTP calls wrap a 15s-per-attempt timeout with exponential backoff + jitter (3 retries on 408/429/5xx), and `gemini-cli` subprocess calls retry only on exit codes 1/124/130 or `DeadlineExceeded` (2 retries). JSON parse failures are deliberately *not* retried — they fall to the next batch run instead.
- **Worker pool, not sequential loop.** Extractor and ocr-worker use `errgroup.SetLimit` (default 3) so a single hung `gemini-cli` call cannot stall the whole batch.
- **Matching deliberately left simple.** The matcher is a direct comparison rather than an LLM-ranked one, so whether to add an LLM there can be decided from real matching output rather than from speculation.

## Repository Layout

```
cmd/
  crawler/             gamejob crawler (HTML bodies + image URLs)
  ocr-worker/          image-bodied postings → OCR text
  extractor/           body text → experience/competency/trait JSON
  pipeline/            runs the three batches in sequence for local dev
  normalize-companies/ one-shot data migration for legal-entity suffixes
  seed-preferences/    seed user preference fixtures
  user-extractor/      derive structured user profile from raw input
  notify/              email notifier for new recommended postings

internal/
  crawler/             scraping orchestration (Functional Options)
  gamejob/             gamejob-specific HTML / iframe parsing
  ocrworker/           OCR worker pipeline
  extractor/           extraction pipeline
  matcher/             posting ↔ user matching
  repository/          GORM repositories
  model/               domain models (postings, bodies, users, preferences)
  geminiexec/          gemini-cli subprocess wrapper
  retry/               retry/backoff helper shared by all batches
  batch/               errgroup-based worker pool
  config/              env-driven configuration
  mailing/             SMTP notifier
  telemetry/           metrics/logging hooks
```

## Getting Started

### Prerequisites

- **Go 1.26+**
- **MySQL** running locally (the pipeline auto-migrates the schema on first run)
- **[`gemini` CLI](https://github.com/google-gemini/gemini-cli)** on `PATH` — used by the OCR worker and extractor
- A **`.env`** file in the repo root (git-ignored). Minimum keys:

  ```env
  DB_HOST=127.0.0.1
  DB_PORT=3306
  DB_USER=root
  DB_PASS=yourpassword
  DB_NAME=skipjd
  DB_AUTO_MIGRATE=true

  # only needed for `cmd/notify`
  SMTP_HOST=smtp.gmail.com
  SMTP_PORT=587
  SMTP_USER=you@example.com
  SMTP_PASS=app-password
  MAIL_FROM=you@example.com
  MAIL_TO=you@example.com
  ```

### Run the whole pipeline

```sh
# crawl → OCR → extract, in one process
go run ./cmd/pipeline
```

Handy flags: `-limit N` (postings per OCR/extract batch, default 50), `-workers N`
(concurrency per stage, default 3), and `-skip-crawl` / `-skip-ocr` / `-skip-extract`
to run a subset.

### Run a single stage

```sh
go run ./cmd/crawler      # collect postings + image URLs
go run ./cmd/ocr-worker   # OCR image-bodied postings   (-limit, -workers)
go run ./cmd/extractor    # body text → structured JSON  (-limit, -workers)
```

### Recommend & notify

```sh
# seed a user's preferences (omit --apply for a dry-run)
go run ./cmd/seed-preferences --user-id 1 --duty-codes 1,3 --apply

# email the top recommendations (--dry-run prints to stdout instead of sending)
go run ./cmd/notify --user-id 1 --dry-run
```

