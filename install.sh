#!/bin/sh
set -eu
cd "$(dirname "$0")"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing $1. Install it, then run ./install.sh again."
    exit 1
  fi
}

need_cmd docker
if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose plugin is missing. On Ubuntu: apt-get install -y docker-compose-v2"
  exit 1
fi

free_port_53() {
  if ! command -v ss >/dev/null 2>&1; then
    return 0
  fi
  if ! ss -ulpn 2>/dev/null | grep -qE '[:.]53\s'; then
    if ! ss -tlpn 2>/dev/null | grep -qE '[:.]53\s'; then
      return 0
    fi
  fi
  if ss -ulpn 2>/dev/null | grep -q 'systemd-resolve' || ss -tlpn 2>/dev/null | grep -q 'systemd-resolve'; then
    echo "Port 53 is held by systemd-resolved. Freeing the stub listener..."
    mkdir -p /etc/systemd/resolved.conf.d
    printf '%s\n' '[Resolve]' 'DNSStubListener=no' > /etc/systemd/resolved.conf.d/disable-stub.conf
    systemctl restart systemd-resolved
    if [ -f /run/systemd/resolve/resolv.conf ]; then
      ln -sfn /run/systemd/resolve/resolv.conf /etc/resolv.conf
    fi
    sleep 1
  fi
  if ss -ulpn 2>/dev/null | grep -qE '[:.]53\s' || ss -tlpn 2>/dev/null | grep -qE '[:.]53\s'; then
    echo "Port 53 is still in use. Stop whatever owns it, then run ./install.sh again."
    ss -ulpn 2>/dev/null | awk 'NR==1 || /:53 /' || true
    ss -tlpn 2>/dev/null | awk 'NR==1 || /:53 /' || true
    exit 1
  fi
  echo "Port 53 is free."
}

free_port_53

if [ ! -f .env ]; then
  cp .env.example .env
  echo "Created .env from .env.example"
fi

if grep -Eq '^PUBLIC_IP=[[:space:]]*$' .env; then
  ip=""
  for url in "https://ifconfig.me" "https://api.ipify.org" "https://icanhazip.com"; do
    ip="$(curl -4 -fsS --max-time 5 "$url" 2>/dev/null || true)"
    ip="$(printf '%s' "$ip" | tr -d '[:space:]')"
    case "$ip" in
      *[!0-9.]*) ip="" ;;
      *.*.*.*) break ;;
      *) ip="" ;;
    esac
  done
  if [ -n "$ip" ]; then
    sed -i "s/^PUBLIC_IP=.*/PUBLIC_IP=${ip}/" .env
    echo "Set PUBLIC_IP=${ip}"
  else
    echo "Set PUBLIC_IP in .env to this machine's public IPv4, then run ./install.sh again."
    exit 1
  fi
fi

if ! grep -Eq '^ADMIN_USER=' .env; then
  printf '\nADMIN_USER=admin\n' >> .env
fi

if grep -Eq '^ADMIN_PASSWORD=change-me-now$' .env || grep -Eq '^ADMIN_PASSWORD=[[:space:]]*$' .env; then
  if command -v openssl >/dev/null 2>&1; then
    pw="$(openssl rand -hex 12)"
  else
    pw="$(od -An -tx1 -N 12 /dev/urandom | tr -d ' \n')"
  fi
  if grep -Eq '^ADMIN_PASSWORD=' .env; then
    sed -i "s/^ADMIN_PASSWORD=.*/ADMIN_PASSWORD=${pw}/" .env
  else
    printf 'ADMIN_PASSWORD=%s\n' "$pw" >> .env
  fi
  echo "Generated ADMIN_PASSWORD=${pw}"
  echo "Save this password. It is also in .env."
fi

docker compose up -d --build
docker compose ps

public_ip="$(grep '^PUBLIC_IP=' .env | cut -d= -f2-)"
user="$(grep '^ADMIN_USER=' .env | cut -d= -f2-)"
[ -n "$user" ] || user=admin

echo
echo "Admin page: http://${public_ip}:8080"
echo "Username:   ${user}"
echo "Password:   (see ADMIN_PASSWORD in .env)"
echo
echo "Quick check:"
echo "  dig @${public_ip} cursor.com A +short"
echo "  dig @${public_ip} one.one.one.one A +short"
