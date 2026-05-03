"""OCR batch worker for job postings whose body is stored as images.

Reads JobPostingImage rows whose JobPosting has no OCR-sourced body yet,
extracts text from each image via the locally-installed `gemini` CLI
(headless `-p` mode), and writes the merged text into JobPostingBody.
Failures are logged to stderr; the next batch run retries naturally
because no failure state is persisted.

Usage:
    python main.py [--limit 50] [--min-ocr-chars 20]

Reads DB config from project root .env (shared with the Go crawler):
    DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME, REQUIRE_DB_TLS

Requires `gemini` (gemini-cli) on PATH and an authenticated session
(OAuth or GEMINI_API_KEY).
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

import requests
from dotenv import load_dotenv
from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Integer,
    String,
    Text,
    create_engine,
    select,
)
from sqlalchemy.orm import DeclarativeBase, Session

SOURCE_HTML = "html"
SOURCE_OCR = "ocr"


class Base(DeclarativeBase):
    pass


class JobPosting(Base):
    __tablename__ = "job_postings"
    id = Column(Integer, primary_key=True)
    last_seen_at = Column(DateTime)


class JobPostingBody(Base):
    __tablename__ = "job_posting_bodies"
    id = Column(Integer, primary_key=True)
    job_posting_id = Column(Integer, nullable=False, unique=True)
    text = Column(Text, nullable=False)
    source = Column(String(16), nullable=False)
    ready_for_llm = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)


class JobPostingImage(Base):
    __tablename__ = "job_posting_images"
    id = Column(Integer, primary_key=True)
    job_posting_id = Column(Integer, nullable=False, index=True)
    image_url = Column(Text, nullable=False)
    order_index = Column(Integer, nullable=False)


@dataclass
class PostingJob:
    posting_id: int
    image_urls: list[str]


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def build_engine() -> "Engine":
    project_root = Path(__file__).resolve().parents[2]
    load_dotenv(project_root / ".env")

    user = require_env("DB_USER")
    password = require_env("DB_PASS")
    host = require_env("DB_HOST")
    port = require_env("DB_PORT")
    name = require_env("DB_NAME")
    require_tls = os.getenv("REQUIRE_DB_TLS", "false").lower() == "true"

    ssl_arg = "?ssl=true" if require_tls else ""
    dsn = f"mysql+pymysql://{user}:{password}@{host}:{port}/{name}{ssl_arg}"
    return create_engine(dsn, pool_pre_ping=True)


def require_env(key: str) -> str:
    value = os.getenv(key)
    if not value:
        raise RuntimeError(f"missing required env: {key}")
    return value


def fetch_pending_jobs(session: Session, limit: int, offset: int = 0) -> list[PostingJob]:
    rows = session.execute(
        select(JobPostingImage.job_posting_id, JobPosting.last_seen_at)
        .distinct()
        .outerjoin(
            JobPostingBody,
            (JobPostingBody.job_posting_id == JobPostingImage.job_posting_id)
            & (JobPostingBody.source == SOURCE_OCR),
        )
        .join(JobPosting, JobPosting.id == JobPostingImage.job_posting_id)
        .where(JobPostingBody.id.is_(None))
        .order_by(JobPosting.last_seen_at.desc())
        .offset(offset)
        .limit(limit)
    ).all()
    posting_ids = [row[0] for row in rows]

    if not posting_ids:
        return []

    rows = session.execute(
        select(JobPostingImage)
        .where(JobPostingImage.job_posting_id.in_(posting_ids))
        .order_by(JobPostingImage.job_posting_id, JobPostingImage.order_index)
    ).scalars().all()

    grouped: dict[int, list[str]] = {}
    for row in rows:
        grouped.setdefault(row.job_posting_id, []).append(row.image_url)

    return [PostingJob(posting_id=pid, image_urls=grouped[pid]) for pid in posting_ids if pid in grouped]


_BROWSER_USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

# Only fetch images from hosts we trust. Other domains in job_posting_images
# (playwith welfare icons, xlgames banners, etc.) are not posting bodies and
# loading arbitrary external URLs widens the SSRF surface unnecessarily.
_ALLOWED_HOST_SUFFIX = "gamejob.co.kr"
_REFERER = "https://www.gamejob.co.kr/"

_IMAGE_MAGIC = (
    b"\xff\xd8\xff",
    b"\x89PNG\r\n\x1a\n",
    b"GIF87a",
    b"GIF89a",
    b"RIFF",
    b"BM",
)


def _host_allowed(url: str) -> bool:
    from urllib.parse import urlsplit

    host = (urlsplit(url).hostname or "").lower()
    return host == _ALLOWED_HOST_SUFFIX or host.endswith("." + _ALLOWED_HOST_SUFFIX)


def download_image(url: str, timeout: float = 15.0) -> bytes | None:
    if not _host_allowed(url):
        log(f"download skipped url={url} reason=host_not_allowed")
        return None
    headers = {
        "User-Agent": _BROWSER_USER_AGENT,
        "Accept": "image/avif,image/webp,image/apng,image/*,*/*;q=0.8",
        "Referer": _REFERER,
    }
    try:
        resp = requests.get(url, timeout=timeout, headers=headers)
    except Exception as exc:
        log(f"download error url={url} err={exc}")
        return None
    if resp.status_code != 200:
        log(f"download failed url={url} status={resp.status_code}")
        return None
    content_type = (resp.headers.get("Content-Type") or "").split(";", 1)[0].strip().lower()
    payload = resp.content
    if not content_type.startswith("image/") and not payload.startswith(_IMAGE_MAGIC):
        log(f"download non_image url={url} ct={content_type or 'missing'} bytes={len(payload)}")
        return None
    return payload


_GEMINI_PROMPT = (
    "이 이미지에 보이는 텍스트(한국어/영어 포함)를 모두 추출해서 "
    "줄 단위로 한 줄에 하나씩 출력해줘. 설명, 마크다운, 메타정보 없이 텍스트만. "
    "표는 줄 단위로 풀어서 적어줘."
)
# gemini-cli internally retries on 429 with backoff for several minutes; cap
# the per-image wall time so a stuck call cannot stall the whole batch.
_GEMINI_TIMEOUT_SECONDS = 600


def ocr_image(img_path: str) -> str:
    img_abs = os.path.abspath(img_path)
    img_dir = os.path.dirname(img_abs)
    img_name = os.path.basename(img_abs)
    cmd = ["gemini", "-p", f"{_GEMINI_PROMPT} @{img_name}", "-o", "text"]
    try:
        proc = subprocess.run(
            cmd,
            cwd=img_dir,
            capture_output=True,
            text=True,
            timeout=_GEMINI_TIMEOUT_SECONDS,
        )
    except FileNotFoundError:
        log("ocr gemini not found — install gemini-cli and ensure it is on PATH")
        return ""
    except subprocess.TimeoutExpired:
        log(f"ocr gemini timeout img={img_name} after={_GEMINI_TIMEOUT_SECONDS}s")
        return ""

    if proc.returncode != 0:
        stderr_tail = "\n".join((proc.stderr or "").strip().splitlines()[-3:])
        log(f"ocr gemini exit={proc.returncode} img={img_name} stderr={stderr_tail!r}")
        return ""

    text = (proc.stdout or "").strip()
    if not text:
        log(f"ocr gemini empty stdout img={img_name}")
    return text


def process_posting(job: PostingJob, min_chars: int, debug_dir: str | None = None) -> str:
    import tempfile

    parts: list[str] = []

    if debug_dir:
        os.makedirs(debug_dir, exist_ok=True)

    for i, url in enumerate(job.image_urls):
        payload = download_image(url)
        if payload is None:
            continue

        if debug_dir:
            base_name = f"{job.posting_id}_{i}"
            img_path = os.path.join(debug_dir, f"{base_name}.jpg")
            txt_path = os.path.join(debug_dir, f"{base_name}.txt")

            with open(img_path, "wb") as f:
                f.write(payload)

            text = ocr_image(img_path).strip()

            with open(txt_path, "w", encoding="utf-8") as f:
                f.write(text)
        else:
            with tempfile.NamedTemporaryFile(suffix=".jpg", delete=False) as tmp:
                tmp.write(payload)
                tmp_path = tmp.name

            try:
                text = ocr_image(tmp_path).strip()
            finally:
                if os.path.exists(tmp_path):
                    os.remove(tmp_path)

        if len(text) < min_chars:
            log(f"skip ocr_text_too_short posting_id={job.posting_id} url={url} chars={len(text)}")
            continue
        parts.append(text)
    return "\n\n".join(parts).strip()


def upsert_body(session: Session, posting_id: int, ocr_text: str) -> None:
    existing = session.execute(
        select(JobPostingBody).where(JobPostingBody.job_posting_id == posting_id)
    ).scalar_one_or_none()
    now = datetime.now(timezone.utc)

    if existing is None:
        session.add(JobPostingBody(
            job_posting_id=posting_id,
            text=ocr_text,
            source=SOURCE_OCR,
            ready_for_llm=True,
            created_at=now,
            updated_at=now,
        ))
        return

    if existing.source == SOURCE_HTML:
        existing.text = f"{existing.text}\n\n[OCR]\n{ocr_text}"
    else:
        existing.text = ocr_text
    existing.source = SOURCE_OCR
    existing.ready_for_llm = True
    existing.updated_at = now


def run(limit: int, min_chars: int, debug_dir: str | None = None, offset: int = 0) -> None:
    engine = build_engine()
    with Session(engine) as session:
        jobs = fetch_pending_jobs(session, limit, offset)
        log(f"pending postings={len(jobs)}")

        success = 0
        empty = 0
        for job in jobs:
            ocr_text = process_posting(job, min_chars, debug_dir)
            if not ocr_text:
                empty += 1
                log(f"empty result posting_id={job.posting_id}")
                continue
            try:
                upsert_body(session, job.posting_id, ocr_text)
                session.commit()
                success += 1
            except Exception as exc:
                session.rollback()
                log(f"upsert failed posting_id={job.posting_id} err={exc}")

        log(f"done success={success} empty={empty} total={len(jobs)}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--limit", type=int, default=50, help="max postings per run")
    parser.add_argument("--offset", type=int, default=0, help="skip the first N postings (useful for debugging)")
    parser.add_argument("--min-ocr-chars", type=int, default=20,
                        help="discard image OCR result shorter than this many chars")
    parser.add_argument("--debug-dir", type=str, default=None,
                        help="directory to save downloaded images and OCR texts for debugging")
    args = parser.parse_args()

    run(limit=args.limit, min_chars=args.min_ocr_chars, debug_dir=args.debug_dir, offset=args.offset)


if __name__ == "__main__":
    main()
