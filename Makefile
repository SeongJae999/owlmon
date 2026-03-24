.PHONY: dev stop lint format test up down clean

# 개발 환경 전체 시작 (OS 자동 감지)
dev:
ifeq ($(OS),Windows_NT)
	powershell -File dev.ps1
else
	bash dev.sh
endif

# 개발 환경 종료
stop:
ifeq ($(OS),Windows_NT)
	powershell -File stop.ps1
else
	bash stop.sh
endif

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
