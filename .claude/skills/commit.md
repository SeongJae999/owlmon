---
name: commit
description: Conventional Commits 규칙에 따라 커밋 메시지를 작성하고 커밋을 수행합니다
user_invocable: true
---

# Commit Skill

변경사항을 분석하고 Conventional Commits 규칙에 따라 커밋합니다.

## 커밋 메시지 형식

```
<type>: <subject>

<body>
```

## Type 목록

| Type | 설명 | 예시 |
|------|------|------|
| `feat` | 새로운 기능 추가 | `feat(api): add user profile endpoint` |
| `fix` | 버그 수정 | `fix(login): resolve token expiration issue` |
| `docs` | 문서 변경 (코드 변경 없음) | `docs(readme): update installation guide` |
| `style` | 포맷팅, 세미콜론 등 (로직 변경 없음) | `style: fix indentation in config file` |
| `refactor` | 기능 변경 없는 코드 리팩토링 | `refactor(db): simplify query builder logic` |
| `test` | 테스트 추가/수정 | `test(auth): add unit tests for JWT validation` |
| `chore` | 빌드, 설정, 패키지 등 잡일 | `chore: upgrade Go to 1.22` |
| `perf` | 성능 개선 | `perf(search): add index for faster lookups` |
| `ci` | CI/CD 설정 변경 | `ci: add GitHub Actions workflow for lint` |
| `build` | 빌드 시스템, 외부 의존성 변경 | `build: migrate from webpack to vite` |
| `revert` | 이전 커밋 되돌리기 | `revert: revert feat(auth) commit abc1234` |

## 규칙

1. **subject**: 한국어, 마침표 없음
2. **body**: 선택사항, "왜" 변경했는지 설명, 한국어
3. **scope 사용 안 함**: `feat(api):` 같은 괄호 scope 붙이지 않음

## 수행 절차

1. `git status`와 `git diff`로 변경사항 확인
2. 변경 내용 분석 후 적절한 type과 scope 선택
3. 커밋 메시지 초안을 사용자에게 제안
4. 승인 후 커밋 수행
