#!/bin/sh
set -e

INSTALL_DIR="${INSTALL_DIR:-/opt/updara}"
COMPOSE_URL="https://raw.githubusercontent.com/updara-dev/updara/main/docker-compose.dist.yml"

if [ -z "$UPDARA_PUBLIC_URL" ]; then
  IP=$(hostname -I 2>/dev/null | awk '{print $1}')
  echo ""
  echo "Updara Server Install"
  echo "====================="
  echo ""
  printf "Server IP or hostname [detected: %s]: " "$IP"
  read -r INPUT
  [ -n "$INPUT" ] && IP="$INPUT"
  UPDARA_PUBLIC_URL="http://$IP:8080"
fi

echo ""
echo "  Install dir : $INSTALL_DIR"
echo "  Public URL  : $UPDARA_PUBLIC_URL"
echo ""

mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

curl -fsSL "$COMPOSE_URL" -o docker-compose.dist.yml
echo "UPDARA_PUBLIC_URL=$UPDARA_PUBLIC_URL" > .env

docker compose -f docker-compose.dist.yml pull
docker compose -f docker-compose.dist.yml up -d

echo ""
echo "Waiting for server to start..."
sleep 6

TOKEN=$(docker compose -f docker-compose.dist.yml logs server 2>/dev/null \
  | grep "UPDARA TOKEN:" | tail -1 | awk '{print $NF}')

FRONTEND_URL=$(echo "$UPDARA_PUBLIC_URL" | sed 's/:8080/:4000/')

echo ""
echo "=========================================="
echo "Updara is running!"
echo ""
echo "  Frontend : $FRONTEND_URL"
echo "  API      : $UPDARA_PUBLIC_URL"
if [ -n "$TOKEN" ]; then
  echo "  Token    : $TOKEN"
else
  echo "  Token    : docker compose -f $INSTALL_DIR/docker-compose.dist.yml logs server | grep 'UPDARA TOKEN'"
fi
echo "=========================================="
