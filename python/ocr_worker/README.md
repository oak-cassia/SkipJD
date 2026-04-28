# OCR worker

gemini-cli (`gemini -p`) 기반 배치 워커. 크롤러가 큐잉한 `job_posting_images` 행 중 아직 OCR된 본문이 없는 공고를 골라 이미지 텍스트를 추출하고 `job_posting_bodies`에 저장한다.

## 설치

### 1. gemini-cli (PATH 바이너리)

`main.py`는 `gemini` 명령을 subprocess로 호출한다. 호스트 머신에 설치되어 있어야 한다.

```sh
npm i -g @google/gemini-cli   # 또는 사용자 환경의 다른 설치 경로
which gemini
gemini --version
```

인증은 둘 중 하나:

- **OAuth (무료 quota)** — `gemini` 첫 실행 시 브라우저 플로우. cron 환경에서는 토큰 만료 시 무인 갱신이 안 되므로, 만료 신호(stderr `auth` 류 메시지)가 보이면 인터랙티브 셸에서 한 번 더 실행해 갱신한다.
- **API 키** — `GEMINI_API_KEY=...` 환경변수.

### 2. Python 의존성

```sh
cd python/ocr_worker
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

`requirements.txt`는 `SQLAlchemy`, `PyMySQL`, `requests`, `python-dotenv` 네 줄이 전부다.

## 실행

```sh
python main.py                        # limit 50, min-ocr-chars 20 (기본)
python main.py --limit 200            # 운영 권고치
python main.py --limit 1 --debug-dir ./tmp_debug   # 단일 포스팅 디버깅
```

DB 접속 정보는 프로젝트 루트의 `.env`에서 자동 로드 (Go 크롤러와 공유):
`DB_USER`, `DB_PASS`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `REQUIRE_DB_TLS`.

## 동작

1. `job_posting_images`가 존재하면서 `job_posting_bodies.source = "ocr"` 행이 없는 공고 조회 (`last_seen_at DESC`)
2. 각 이미지 다운로드 → gemini-cli (`@basename` 참조, `cwd=image_dir`, `-o text`, 600s 타임아웃) → 결과 글자 수 < `--min-ocr-chars`면 폐기 (로고/장식 필터)
3. 살아남은 결과를 등장 순서대로 합쳐 `job_posting_bodies`에 저장
   - 기존 행 없음 → INSERT (`source="ocr"`)
   - 기존 행 있음(`source="html"`) → 텍스트 append + `source="ocr"`로 UPDATE
4. 실패는 stderr 로그만, 다음 배치 실행이 곧 재시도 (DB에 실패 상태 저장 안 함)

## 운영 / 실패 모드

- **호스트 allowlist**: 다운로드는 `gamejob.co.kr` 및 그 서브도메인만 허용. 그 외 호스트(playwith welfare 아이콘, xlgames 배너 등)는 SSRF 표면을 줄이려고 차단.
- **per-image 실패 격리**: 한 이미지에서 다운로드/OCR 실패가 나도 같은 포스팅의 나머지 이미지는 계속 처리한다.
- **gemini 429**: gemini-cli가 내부적으로 ~10분간 백오프 재시도한다. 그래도 멈추지 않는 경우를 위해 per-image wall-time을 600초로 캡한다.
- **무료 OAuth quota** 가늠치: gemini-2.5-pro 무료 OAuth는 1,000 req/day · 60 RPM. 포스팅당 평균 2–3개 이미지로 잡으면 일일 `--limit 200`이 400–600 req로 안전 마진(quota의 40–60%) 안에 들어간다. 더 키우려면 `GEMINI_API_KEY` 전환을 검토.

## cron 등록 예시

매일 새벽 3시 실행:

```cron
0 3 * * * cd /path/to/skipjd && /path/to/.venv/bin/python python/ocr_worker/main.py --limit 200 >> /var/log/ocr_worker.log 2>&1
```

OAuth 사용 시 토큰 만료가 cron에서 무인 복구되지 않는다는 점에 주의.
