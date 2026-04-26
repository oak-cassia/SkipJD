"""OCR batch worker for job postings whose body is stored as images.

Reads JobPostingImage rows whose JobPosting has no OCR-sourced body yet,
runs PaddleOCR on each image, and writes the merged text into
JobPostingBody. Failures are logged to stderr; the next batch run retries
naturally because no failure state is persisted.

Usage:
    python main.py [--limit 50] [--min-ocr-chars 20]

Reads DB config from project root .env (shared with the Go crawler):
    DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME, REQUIRE_DB_TLS
"""

from __future__ import annotations

import argparse
import io
import os
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Iterable

import requests
from dotenv import load_dotenv
from sqlalchemy import (
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


def fetch_pending_jobs(session: Session, limit: int) -> list[PostingJob]:
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


def download_image(url: str, timeout: float = 15.0) -> bytes | None:
    try:
        resp = requests.get(url, timeout=timeout)
        if resp.status_code != 200:
            log(f"download failed url={url} status={resp.status_code}")
            return None
        return resp.content
    except Exception as exc:
        log(f"download error url={url} err={exc}")
        return None


def ocr_image(ocr, payload: bytes) -> str:
    import numpy as np
    from PIL import Image, UnidentifiedImageError

    try:
        image = Image.open(io.BytesIO(payload)).convert("RGB")
    except (UnidentifiedImageError, OSError) as exc:
        log(f"image decode failed err={exc}")
        return ""
    arr = np.array(image)
    try:
        result = ocr.predict(arr)
    except Exception as exc:
        log(f"ocr predict failed err={exc}")
        return ""
    if not result:
        return ""
    pieces: list[str] = []
    for page in result:
        texts = page.get("rec_texts") or []
        for text in texts:
            if isinstance(text, str) and text.strip():
                pieces.append(text.strip())
    return "\n".join(pieces)


def process_posting(ocr, job: PostingJob, min_chars: int) -> str:
    parts: list[str] = []
    for url in job.image_urls:
        payload = download_image(url)
        if payload is None:
            continue
        text = ocr_image(ocr, payload).strip()
        if len(text) < min_chars:
            log(f"skip ocr_text_too_short posting_id={job.posting_id} url={url} chars={len(text)}")
            continue
        parts.append(text)
    return "\n\n".join(parts).strip()


def upsert_body(session: Session, posting_id: int, ocr_text: str) -> None:
    existing = session.execute(
        select(JobPostingBody).where(JobPostingBody.job_posting_id == posting_id)
    ).scalar_one_or_none()
    now = datetime.utcnow()

    if existing is None:
        session.add(JobPostingBody(
            job_posting_id=posting_id,
            text=ocr_text,
            source=SOURCE_OCR,
            created_at=now,
            updated_at=now,
        ))
        return

    if existing.source == SOURCE_HTML:
        existing.text = f"{existing.text}\n\n[OCR]\n{ocr_text}"
    else:
        existing.text = ocr_text
    existing.source = SOURCE_OCR
    existing.updated_at = now


def run(limit: int, min_chars: int) -> None:
    from paddleocr import PaddleOCR

    log(f"loading paddle ocr model lang=korean")
    ocr = PaddleOCR(use_textline_orientation=False, lang="korean")

    engine = build_engine()
    with Session(engine) as session:
        jobs = fetch_pending_jobs(session, limit)
        log(f"pending postings={len(jobs)}")

        success = 0
        empty = 0
        for job in jobs:
            ocr_text = process_posting(ocr, job, min_chars)
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
    parser.add_argument("--min-ocr-chars", type=int, default=20,
                        help="discard image OCR result shorter than this many chars")
    args = parser.parse_args()

    run(limit=args.limit, min_chars=args.min_ocr_chars)


if __name__ == "__main__":
    main()
