#!/usr/bin/env bash
set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# ── Usage ─────────────────────────────────────────────────────────────
# ./restart.sh          → 풀 리빌드 (프론트엔드 + Go + Docker)
# ./restart.sh quick    → .env 변경만 반영 (컨테이너 재시작, 빌드 없음)
# ./restart.sh frontend → 프론트엔드만 빌드 (Go 재빌드 없음)
# ./restart.sh backend  → Go만 재빌드 (프론트엔드 빌드 스킵)

MODE="${1:-full}"

case "$MODE" in
  quick|env)
    echo "▶ .env 변경 반영 (컨테이너 재시작만)..."
    docker compose restart
    echo "✓ 완료 — .env 변경이 반영되었습니다."
    ;;

  frontend|fe)
    echo "▶ 프론트엔드만 빌드..."
    cd "$ROOT/web"
    npm run build
    echo "✓ 완료 — web/dist 갱신됨 (볼륨 마운트로 자동 반영)"
    echo "  브라우저에서 새로고침하세요."
    ;;

  backend|be)
    echo "▶ Go 백엔드만 재빌드..."
    docker compose down
    docker compose up -d --build
    echo "✓ 완료"
    ;;

  full|"")
    echo "▶ 기존 컨테이너 종료..."
    docker compose down

    echo "▶ 프론트엔드 빌드..."
    cd "$ROOT/web"
    npm install --silent
    npm run build

    echo "▶ Docker 이미지 빌드 + 실행..."
    cd "$ROOT"
    docker compose up -d --build
    echo "✓ 풀 리빌드 완료"
    ;;

  *)
    echo "사용법: ./restart.sh [quick|frontend|backend|full]"
    echo ""
    echo "  quick     .env 변경만 반영 (재시작, 빌드 없음) — 2초"
    echo "  frontend  프론트엔드만 빌드 — 5초"
    echo "  backend   Go만 재빌드 — 30초"
    echo "  full      전체 재빌드 (기본값) — 60초"
    exit 1
    ;;
esac

echo ""
echo "실행 확인:"
echo "  - 브라우저:  http://localhost:8080"
echo "  - API 상태: curl http://localhost:8080/api/status"
echo "  - 헬스체크: curl http://localhost:8080/health"

if [ "$MODE" != "frontend" ] && [ "$MODE" != "fe" ]; then
  echo ""
  echo "▶ 로그 출력 (Ctrl+C로 종료)..."
  docker compose logs -f
fi
