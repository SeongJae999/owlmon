.PHONY: install dev lint format test up down clean

# 의존성 설치
install:
	pip install -e ".[all]"

# 개발 환경 전체 세팅
dev: install up
	@echo "개발 환경 준비 완료! http://localhost:3000 (Grafana)"

# 린트
lint:
	ruff check src/ tests/
	mypy src/

# 포맷
format:
	ruff format src/ tests/
	ruff check --fix src/ tests/

# 테스트
test:
	pytest --cov=src --cov-report=term-missing

# Docker 인프라 시작
up:
	docker compose up -d

# Docker 인프라 중지
down:
	docker compose down

# 전체 정리 (볼륨 포함)
clean:
	docker compose down -v
