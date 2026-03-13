#!/usr/bin/env bash
set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "▶ 기존 컨테이너 종료..."
docker compose -f "$ROOT/docker-compose.yml" down

echo "▶ 프론트엔드 빌드..."
cd "$ROOT/web"
npm install --silent
npm run build

echo "▶ Docker 이미지 빌드 + 실행..."
cd "$ROOT"
docker compose up -d --build

echo "▶ 로그 출력 (Ctrl+C로 종료)..."
echo ""
echo "실행 확인:"
echo "  - 브라우저:  http://localhost:8080"
echo "  - API 상태: curl http://localhost:8080/api/status"
echo "  - 헬스체크: curl http://localhost:8080/health"
echo ""
docker compose logs -f
