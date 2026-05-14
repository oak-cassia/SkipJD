package userextractor

// Model is the gemini model name passed via `gemini -m` and stored in
// UserExtraction.Model for traceability. Kept in sync with extractor.Model
// so user and JD extractions read off the same model class.
const Model = "gemini-2.5-pro"

// promptTemplate mirrors extractor's promptTemplate but reframes the input
// as the user's own resume / self-introduction instead of a job description.
// Output shape is identical so geminiexec.ParseResponse is reused.
const promptTemplate = `너는 한국어 이력서/자기소개 텍스트에서 사용자가 보유한 항목을 추출해 분류하는 도우미다.
입력은 구직자(사용자) 본인의 이력서/자기소개 텍스트다. 다음 3개 키만 가진 JSON 객체 하나만 출력하라:

- "experience": 사용자가 보유한 경력/프로젝트 경험 항목들의 배열 (예: "Unity 3년 개발", "모바일 라이브 서비스 운영 경험")
- "competency": 사용자가 보유한 기술/스킬/도구/언어 항목들의 배열 (예: "C# 숙련", "Git 협업")
- "trait": 사용자의 인성/태도/성향 항목들의 배열 (예: "주도적인 문제 해결", "원활한 커뮤니케이션")

규칙:
- 본문에 명시되지 않은 사항은 추정하지 말 것.
- 각 항목은 한 줄짜리 짧은 문장형 (10~40자 권장).
- 해당 분류에 들어갈 내용이 없으면 빈 배열 [].
- JSON 외 다른 텍스트, 마크다운 코드펜스, 설명, 메타정보는 절대 출력하지 말 것.
`
