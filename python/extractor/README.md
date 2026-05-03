# Extractor worker

gemini-cli (`gemini -p`) 기반 본문 정제 워커. `job_posting_bodies.ready_for_llm = true`인 본문을 LLM에 통과시켜 **경험 / 역량 / 특성** 3분류로 구조화하고 `job_posting_extractions`에 저장한다. 원본 본문은 보존된다.

## 설치

### 1. gemini-cli (PATH 바이너리)

`main.py`는 `gemini` 명령을 subprocess로 호출한다. 호스트 머신에 설치되어 있어야 한다 (ocr_worker와 공유).

```sh
npm i -g @google/gemini-cli
which gemini
gemini --version
```

인증은 OAuth 또는 `GEMINI_API_KEY` 환경변수.

### 2. Python 의존성 (uv)

```sh
brew install uv
# 또는: curl -LsSf https://astral.sh/uv/install.sh | sh
```

`pyproject.toml` 기반으로 `uv`가 가상환경/패키지를 자동 관리한다.

## 실행

```sh
cd python/extractor
uv run main.py                        # limit 50 (기본)
uv run main.py --limit 100            # 운영 권고치
uv run main.py --limit 1 --debug-dir ./tmp_debug   # 단일 포스팅 디버깅 (본문/응답 보관)
```

DB 접속 정보는 프로젝트 루트 `.env` 공유: `DB_USER`, `DB_PASS`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `REQUIRE_DB_TLS`.

## 동작

1. 다음 조건을 만족하는 `job_posting_bodies` 행 조회:
   - `ready_for_llm = true`
   - `job_posting_extractions` 행이 없거나, `source_body_updated_at < body.updated_at` (본문 갱신 후 재추출)
   - 정렬: `updated_at DESC`, `--limit` / `--offset` 적용
2. 각 본문을 임시 파일에 쓰고 `gemini -m gemini-2.5-pro -p "<프롬프트> @body.txt" -o text`로 호출 (cwd=임시 디렉터리, 600s 타임아웃)
3. stdout을 JSON으로 파싱 — 코드 펜스 자동 제거. 키 누락 / 타입 오류 시 row를 fail로 기록하고 다음 본문 진행
4. 결과를 `job_posting_extractions`에 upsert (insert 또는 update). `source_body_updated_at`을 본문 시점으로 기록해서 다음 실행이 중복 작업 안 하도록 함
5. 실패는 stderr 로그만 남기고 다음 배치에서 자연스럽게 재시도

## ready_for_llm 흐름

크롤링 시점에 다음 규칙으로 `ready_for_llm` 컬럼이 결정된다 — 이 워커는 그 결과만 본다.

| 본문 종류 | 같은 공고에 이미지 | `ready_for_llm` | 설정 위치 |
|---|---|---|---|
| HTML | 없음 | `true` | Go 크롤러 `UpsertJobPostingBodyHTML` |
| HTML | 있음 (OCR 대기) | `false` | Go 크롤러 `UpsertJobPostingBodyHTML` |
| OCR (이미지→텍스트 완료) | — | `true` | Python `ocr_worker.upsert_body` |
| HTML + OCR 병합 | — | `true` | Python `ocr_worker.upsert_body` |

기존 행 백필이 필요하면 1회 실행:

```sql
UPDATE job_posting_bodies SET ready_for_llm = 1 WHERE source = 'ocr';

UPDATE job_posting_bodies b
LEFT JOIN job_posting_images i ON i.job_posting_id = b.job_posting_id
SET b.ready_for_llm = 1
WHERE b.source = 'html' AND i.id IS NULL;
```

## 검증

```sh
uv run main.py --limit 5 --debug-dir /tmp/extract-debug
```

기대 stderr:

```
pending postings=5
done success=5 failed=0 total=5
```

샘플 결과 검사:

```sql
SELECT jp.title,
       JSON_LENGTH(e.experience) AS exp_n,
       JSON_LENGTH(e.competency) AS comp_n,
       JSON_LENGTH(e.trait)      AS trait_n,
       JSON_EXTRACT(e.experience, '$[0]') AS exp_first
FROM job_posting_extractions e
JOIN job_postings jp ON jp.id = e.job_posting_id
ORDER BY e.updated_at DESC
LIMIT 5;
```

## cron 예시

매일 새벽 4시 (ocr_worker 03시 이후로):

```cron
0 4 * * * cd /path/to/skipjd/python/extractor && uv run main.py --limit 200 >> /var/log/extractor.log 2>&1
```
