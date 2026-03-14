# TODO

## 1단계: MVP
- [ ] GORM 모델 작성
    - [ ] `User`
    - [ ] `AlertSetting`
    - [ ] `JobPosting`
    - [ ] `SentJobAlert`
- [ ] 회원가입 API 만들기
- [ ] 알림 조건 저장 API 만들기
- [ ] Worker 엔트리포인트 만들기
- [ ] 정기 배치 구조 만들기
- [ ] 공고 수집 로직 붙이기
- [ ] 조건 매칭 로직 만들기
- [ ] 메일 발송 붙이기
- [ ] 중복 발송 방지 처리

## 2단계: 로컬 운영
- [ ] `.env` 로드 연결 (`godotenv`)
- [ ] Dockerfile 작성
- [ ] `docker-compose.yml` 작성
    - [ ] api
    - [ ] worker
    - [ ] mysql
- [ ] 로컬에서 api/worker/mysql 함께 실행

## 3단계: 클라우드 맛보기
- [ ] AWS EC2 배포
- [ ] Terraform 설치
- [ ] Terraform으로 EC2/보안그룹 생성
- [ ] 앱 배포

## 4단계: 서비스답게
- [ ] RDS 분리
- [ ] HTTPS 적용
- [ ] 로그/모니터링 정리
- [ ] 운영 설정 분리