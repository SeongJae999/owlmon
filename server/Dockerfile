# OWLmon 서버 멀티스테이지 빌드
# Build: golang:1.26 alpine 이미지에서 빌드
# Run:   alpine 최소 이미지에서 실행 (수십 MB 수준)

# ─── 1단계: 빌드 ───────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# 의존성 캐시 (go.mod/go.sum만 먼저 복사)
COPY go.mod go.sum ./
RUN go mod download

# 소스 복사 후 빌드
COPY . .
# CGO 비활성화로 정적 바이너리 생성 (alpine에서 실행하려면 필수)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/owlmon-server .

# ─── 2단계: 실행 ───────────────────────────────────────
FROM alpine:3.20

# TLS 인증서 + 한국 시간대
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Seoul

# 비루트 사용자
RUN addgroup -S owlmon && adduser -S -G owlmon owlmon

WORKDIR /app
COPY --from=builder /out/owlmon-server /app/owlmon-server

# JWT secret 영속화용 디렉토리
RUN mkdir -p /app/data && chown -R owlmon:owlmon /app
VOLUME ["/app/data"]

USER owlmon
EXPOSE 8080

ENTRYPOINT ["/app/owlmon-server"]
