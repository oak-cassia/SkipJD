"""LLM extraction worker: classify job posting bodies into experience / competency / trait.

Reads JobPostingBody rows whose `ready_for_llm` flag is true and whose
JobPostingExtraction is missing or stale, calls the locally-installed
`gemini` CLI with a structured prompt, parses the JSON response, and
writes one row per posting into job_posting_extractions.

Failures are logged to stderr; no failure state is persisted, so the next
batch run retries naturally.

Usage:
    uv run main.py [--limit 50] [--offset 0] [--debug-dir DIR]

Reads DB config from project root .env (shared with the Go crawler):
    DB_USER, DB_PASS, DB_HOST, DB_PORT, DB_NAME, REQUIRE_DB_TLS

Requires `gemini` (gemini-cli) on PATH and an authenticated session
(OAuth or GEMINI_API_KEY).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

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

GEMINI_MODEL = "gemini-2.5-pro"


class Base(DeclarativeBase):
    pass


class JobPostingBody(Base):
    __tablename__ = "job_posting_bodies"
    id = Column(Integer, primary_key=True)
    job_posting_id = Column(Integer, nullable=False, unique=True)
    text = Column(Text, nullable=False)
    source = Column(String(16), nullable=False)
    ready_for_llm = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)


class JobPostingExtraction(Base):
    __tablename__ = "job_posting_extractions"
    id = Column(Integer, primary_key=True)
    job_posting_id = Column(Integer, nullable=False, unique=True)
    experience = Column(Text, nullable=False)
    competency = Column(Text, nullable=False)
    trait = Column(Text, nullable=False)
    model = Column(String(64), nullable=False)
    source_body_updated_at = Column(DateTime, nullable=False)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)


@dataclass
class PendingBody:
    posting_id: int
    text: str
    body_updated_at: datetime


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def require_env(key: str) -> str:
    value = os.getenv(key)
    if not value:
        raise RuntimeError(f"missing required env: {key}")
    return value


def build_engine():
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


def fetch_pending(session: Session, limit: int, offset: int = 0) -> list[PendingBody]:
    """Bodies that are ready_for_llm AND (no extraction yet OR body has been updated since last extraction)."""
    rows = session.execute(
        select(
            JobPostingBody.job_posting_id,
            JobPostingBody.text,
            JobPostingBody.updated_at,
        )
        .outerjoin(
            JobPostingExtraction,
            JobPostingExtraction.job_posting_id == JobPostingBody.job_posting_id,
        )
        .where(JobPostingBody.ready_for_llm.is_(True))
        .where(
            (JobPostingExtraction.id.is_(None))
            | (JobPostingExtraction.source_body_updated_at < JobPostingBody.updated_at)
        )
        .order_by(JobPostingBody.updated_at.desc())
        .offset(offset)
        .limit(limit)
    ).all()
    return [PendingBody(posting_id=r[0], text=r[1], body_updated_at=r[2]) for r in rows]


_GEMINI_PROMPT = """너는 한국어 채용공고에서 지원자에게 요구되는 항목을 추출해 분류하는 도우미다.
입력은 채용공고 본문 텍스트다. 다음 3개 키만 가진 JSON 객체 하나만 출력하라:

- "experience": 요구/우대 경력에 해당하는 짧은 항목들의 배열 (예: "Unity 3년 이상", "모바일 라이브 서비스 운영 경험")
- "competency": 보유 기술/스킬/도구/언어 항목들의 배열 (예: "C# 숙련", "Git 협업")
- "trait": 인성/태도/성향 항목들의 배열 (예: "주도적인 문제 해결", "원활한 커뮤니케이션")

규칙:
- 본문에 명시되지 않은 사항은 추정하지 말 것.
- 각 항목은 한 줄짜리 짧은 문장형 (10~40자 권장).
- 해당 분류에 들어갈 내용이 없으면 빈 배열 [].
- JSON 외 다른 텍스트, 마크다운 코드펜스, 설명, 메타정보는 절대 출력하지 말 것.
"""

_GEMINI_TIMEOUT_SECONDS = 600
_FENCE_RE = re.compile(r"^```(?:json)?\s*|\s*```$", re.MULTILINE)


def call_gemini(body_text: str, debug_dir: str | None = None, posting_id: int | None = None) -> str:
    """Run gemini-cli on a temp file containing the body. Returns stdout (raw)."""
    if debug_dir and posting_id is not None:
        os.makedirs(debug_dir, exist_ok=True)
        body_path = os.path.join(debug_dir, f"{posting_id}_body.txt")
        with open(body_path, "w", encoding="utf-8") as f:
            f.write(body_text)
        body_dir = debug_dir
        body_name = f"{posting_id}_body.txt"
        cleanup = None
    else:
        tmp = tempfile.NamedTemporaryFile(suffix=".txt", delete=False, mode="w", encoding="utf-8")
        tmp.write(body_text)
        tmp.close()
        body_path = tmp.name
        body_dir = os.path.dirname(body_path)
        body_name = os.path.basename(body_path)
        cleanup = body_path

    cmd = [
        "gemini",
        "--skip-trust",
        "-m", GEMINI_MODEL,
        "-p", f"{_GEMINI_PROMPT}\n\n@{body_name}",
        "-o", "text",
    ]
    try:
        proc = subprocess.run(
            cmd,
            cwd=body_dir,
            capture_output=True,
            text=True,
            timeout=_GEMINI_TIMEOUT_SECONDS,
        )
    except FileNotFoundError:
        log("extract gemini not found — install gemini-cli and ensure it is on PATH")
        return ""
    except subprocess.TimeoutExpired:
        log(f"extract gemini timeout posting_id={posting_id} after={_GEMINI_TIMEOUT_SECONDS}s")
        return ""
    finally:
        if cleanup and os.path.exists(cleanup):
            os.remove(cleanup)

    if proc.returncode != 0:
        stderr_tail = "\n".join((proc.stderr or "").strip().splitlines()[-3:])
        log(f"extract gemini exit={proc.returncode} posting_id={posting_id} stderr={stderr_tail!r}")
        return ""

    return (proc.stdout or "").strip()


def parse_response(raw: str) -> dict[str, list[str]] | None:
    """Strip optional code fences and parse JSON. Returns None if shape is wrong."""
    if not raw:
        return None
    cleaned = _FENCE_RE.sub("", raw).strip()
    try:
        data = json.loads(cleaned)
    except json.JSONDecodeError:
        return None
    if not isinstance(data, dict):
        return None
    out: dict[str, list[str]] = {}
    for key in ("experience", "competency", "trait"):
        value = data.get(key, [])
        if not isinstance(value, list):
            return None
        out[key] = [str(v).strip() for v in value if str(v).strip()]
    return out


def upsert_extraction(
    session: Session,
    posting_id: int,
    result: dict[str, list[str]],
    body_updated_at: datetime,
) -> None:
    existing = session.execute(
        select(JobPostingExtraction).where(JobPostingExtraction.job_posting_id == posting_id)
    ).scalar_one_or_none()
    now = datetime.now(timezone.utc)

    experience_json = json.dumps(result["experience"], ensure_ascii=False)
    competency_json = json.dumps(result["competency"], ensure_ascii=False)
    trait_json = json.dumps(result["trait"], ensure_ascii=False)

    if existing is None:
        session.add(JobPostingExtraction(
            job_posting_id=posting_id,
            experience=experience_json,
            competency=competency_json,
            trait=trait_json,
            model=GEMINI_MODEL,
            source_body_updated_at=body_updated_at,
            created_at=now,
            updated_at=now,
        ))
        return

    existing.experience = experience_json
    existing.competency = competency_json
    existing.trait = trait_json
    existing.model = GEMINI_MODEL
    existing.source_body_updated_at = body_updated_at
    existing.updated_at = now


def run(limit: int, offset: int = 0, debug_dir: str | None = None) -> None:
    engine = build_engine()
    with Session(engine) as session:
        pending = fetch_pending(session, limit, offset)
        log(f"pending postings={len(pending)}")

        success = 0
        failed = 0
        for body in pending:
            raw = call_gemini(body.text, debug_dir=debug_dir, posting_id=body.posting_id)
            result = parse_response(raw)
            if result is None:
                failed += 1
                preview = (raw or "").replace("\n", " ")[:120]
                log(f"parse failed posting_id={body.posting_id} preview={preview!r}")
                continue
            try:
                upsert_extraction(session, body.posting_id, result, body.body_updated_at)
                session.commit()
                success += 1
            except Exception as exc:
                session.rollback()
                failed += 1
                log(f"upsert failed posting_id={body.posting_id} err={exc}")

        log(f"done success={success} failed={failed} total={len(pending)}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--limit", type=int, default=50, help="max postings per run")
    parser.add_argument("--offset", type=int, default=0, help="skip the first N postings (for debugging)")
    parser.add_argument("--debug-dir", type=str, default=None,
                        help="directory to retain body files instead of using temp files")
    args = parser.parse_args()

    run(limit=args.limit, offset=args.offset, debug_dir=args.debug_dir)


if __name__ == "__main__":
    main()
