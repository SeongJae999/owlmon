# OWLmon 웹 프론트엔드 멀티스테이지 빌드
# Build: node 이미지에서 vite build (web/dist 생성)
# Run:   nginx alpine으로 정적 파일 서빙 + /api 프록시

# ─── 1단계: 빌드 ───────────────────────────────────────
FROM node:22-alpine AS builder

WORKDIR /src

# 의존성 캐시
COPY package.json package-lock.json ./
RUN npm ci

# 소스 복사 후 빌드 (vite + tsc)
COPY . .
RUN npm run build

# ─── 2단계: 정적 서빙 ───────────────────────────────────
FROM nginx:1.27-alpine

# 한국 시간대
RUN apk add --no-cache tzdata
ENV TZ=Asia/Seoul

# 빌드 산출물 + nginx 설정
COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
