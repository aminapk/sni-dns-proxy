# SNI DNS Proxy

One Docker service that:

1. Runs a DNS server. Names on your list resolve to this machine. Everything else is forwarded to normal resolvers.
2. Accepts the connections those clients then make, reads the TLS server name or the HTTP `Host` header, and copies the bytes to the real site.
3. Serves an admin page for domains, client IP access, speed limits, and traffic.

TLS is not decrypted. The client still completes TLS with the real site, so the real certificate stays valid.

## Requirements

- A Linux VPS with a public IPv4 address
- Docker Engine + Compose plugin (`docker compose`)
- Free ports **53** (UDP/TCP), **80**, **443**, and **8080**
- Outbound UDP/TCP 53 to upstream resolvers (`1.1.1.1`, `8.8.8.8` by default)

## Quick start (new server)

```bash
git clone <YOUR_REPO_URL> sni-dns-proxy
cd sni-dns-proxy
chmod +x install.sh
./install.sh
```

`install.sh` will:

1. Check that Docker is installed
2. Free port 53 when it is held by systemd-resolved (Ubuntu/Debian)
3. Create `.env` from `.env.example`
4. Detect the public IPv4 when possible
5. Generate an admin password when the placeholder is still set
6. Build and start the container

Then open `http://YOUR_IP:8080` and sign in. Default username is `admin`. The password is printed once and saved in `.env`.

### Install Docker (Ubuntu)

```bash
sudo apt-get update
sudo apt-get install -y docker.io docker-compose-v2
sudo systemctl enable --now docker
```

### Manual start

```bash
cp .env.example .env
# set PUBLIC_IP and ADMIN_PASSWORD (at least 8 characters)
docker compose up -d --build
```

### Free port 53 yourself (if install.sh did not)

On Ubuntu, systemd-resolved often owns port 53:

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
echo -e '[Resolve]\nDNSStubListener=no' | sudo tee /etc/systemd/resolved.conf.d/disable-stub.conf
sudo systemctl restart systemd-resolved
sudo ln -sfn /run/systemd/resolve/resolv.conf /etc/resolv.conf
```

Open the firewall for DNS and the proxy:

```bash
# ufw example
sudo ufw allow 53/tcp
sudo ufw allow 53/udp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 8080/tcp
```

### Verify

```bash
dig @YOUR_IP cursor.com A +short
dig @YOUR_IP one.one.one.one A +short
curl -I http://YOUR_IP:8080/login
```

A listed name should return `YOUR_IP`. An unlisted name should return a real address from upstream DNS.

## Admin

- **Dashboard:** query/proxy counters and recent activity
- **Domains:** searchable, scrollable list; add, disable, delete; check a name
- **Clients:** Open / Locked mode for unknown IPs; per-IP or CIDR rules (`DNS + Proxy`, `DNS only`, `Proxy only`, `Blocked`); optional Mbps caps
- **Traffic:** bytes per client IP for today and all time
- **Account:** change the admin username and password

`ADMIN_USER` / `ADMIN_PASSWORD` create the first admin row only. After that, the account in the database wins.

Domains, client rules, and traffic live in the Docker volume `proxy-data` and survive restarts.

A new database starts with the built-in list in `internal/defaults/domains.txt`. Each name is stored as itself and as `*.name`. Extra names in `SEED_DOMAINS` are added only when the database is empty.

## Point a client at it

Set the device or router DNS server to this machine's IP:

```bash
dig @YOUR_IP example.com A
dig @YOUR_IP example.com AAAA
dig @YOUR_IP one.one.one.one A
```

Listed names get an `A` record of `PUBLIC_IP`. Their `AAAA` and HTTPS answers are empty so clients use IPv4 through this proxy. Unlisted names get the real address from `1.1.1.1` / `8.8.8.8`.

## Domain patterns

| Pattern | Matches |
|---|---|
| `example.com` | that name only |
| `sub.example.com` | that name only |
| `*.example.com` | every subdomain, at any depth |

`*.example.com` does not include `example.com`. Add the apex as its own entry when you want it too.

## Ports

| Port | Role |
|---|---|
| 53/udp and 53/tcp | DNS |
| 80/tcp | HTTP, using the `Host` header |
| 443/tcp | HTTPS, using TLS SNI |
| 8080/tcp | Admin page |

Keep port 8080 off the public internet when you can, or set `ADMIN_ADDR=127.0.0.1:8080` and publish it only to localhost.

## Configuration

Copy `.env.example` to `.env`. Never commit `.env`.

| Variable | Default | Purpose |
|---|---|---|
| `PUBLIC_IP` | required | IPv4 returned for listed names |
| `ADMIN_USER` | `admin` | First-time admin username |
| `ADMIN_PASSWORD` | required, at least 8 characters | First-time admin password |
| `UPSTREAM_DNS` | `1.1.1.1,8.8.8.8` | Resolvers for everything else, and for the proxy's own lookups. IP addresses only. |
| `SEED_DOMAINS` | empty | Extra names added with the built-in list, only when the database is empty |
| `DNS_ADDR` | `:53` | |
| `HTTP_ADDR` | `:80` | |
| `HTTPS_ADDR` | `:443` | |
| `ADMIN_ADDR` | `:8080` | |
| `DATABASE_PATH` | `data/proxy.db` | Compose sets `/data/proxy.db` |
| `LOG_LEVEL` | `info` | `debug` prints handshake failures |

The proxy asks `UPSTREAM_DNS` directly. It does not use the system resolver, so a listed name cannot point back at this server.

## Limits

- The client has to use this DNS. Apps that use their own encrypted DNS ignore it.
- Only ports 80 and 443 are proxied.
- HTTP/3 (QUIC on UDP 443) is not proxied. Browsers fall back to TCP, so the first visit can be slower.
- If a site hides its name with Encrypted Client Hello, the proxy cannot see SNI and closes the connection.
- Testing from the same LAN as the server can fail when hairpin NAT does not work. A phone on mobile data is a reliable check.

## Operations

```bash
docker logs -f sni-dns-proxy
docker compose ps
docker compose restart
docker compose down      # keeps the database volume
docker compose down -v  # deletes the database volume
```

## Development

```bash
go test ./...
```

Local high ports (no root / no port 53):

```bash
PUBLIC_IP=203.0.113.10 \
ADMIN_PASSWORD=test-password \
DNS_ADDR=127.0.0.1:15353 \
HTTP_ADDR=127.0.0.1:18080 \
HTTPS_ADDR=127.0.0.1:18443 \
ADMIN_ADDR=127.0.0.1:18081 \
DATABASE_PATH=./data/dev.db \
go run .
```
