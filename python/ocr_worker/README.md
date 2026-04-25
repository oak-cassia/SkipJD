# OCR worker

PaddleOCR 기반 배치 워커. 크롤러가 큐잉한 `job_posting_images` 행 중 아직 OCR된 본문이 없는 공고를 골라 이미지 텍스트를 추출하고 `job_posting_bodies`에 저장한다.

## 설치

```sh
cd python/ocr_worker
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

PaddlePaddle 설치는 환경(CPU/GPU, OS)에 따라 추가 단계가 필요할 수 있다. 공식 가이드: https://www.paddlepaddle.org.cn/install/quick

## 실행

```sh
python main.py                 # limit 50, 결과 글자 수 < 20 폐기 (기본값)
python main.py --limit 100
python main.py --min-ocr-chars 10
```

DB 접속 정보는 프로젝트 루트의 `.env`에서 자동 로드 (Go 크롤러와 공유):
`DB_USER`, `DB_PASS`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `REQUIRE_DB_TLS`.

## 동작

1. `job_posting_images`가 존재하면서 `job_posting_bodies.source = "ocr"` 행이 없는 공고 조회 (`last_seen_at DESC`)
2. 각 이미지 다운로드 → PaddleOCR → 결과 글자 수 < 임계값이면 폐기 (로고/장식 필터)
3. 살아남은 결과를 등장 순서대로 합쳐 `job_posting_bodies`에 저장
   - 기존 행 없음 → INSERT (source="ocr")
   - 기존 행 있음(source="html") → 텍스트 append + source="ocr"로 UPDATE
4. 실패는 stderr 로그만, 다음 배치 실행이 곧 재시도 (DB에 실패 상태 저장 안 함)

## cron 등록 예시

매일 새벽 3시 실행:

```cron
0 3 * * * cd /path/to/skipjd && /path/to/.venv/bin/python python/ocr_worker/main.py --limit 200 >> /var/log/ocr_worker.log 2>&1
```
