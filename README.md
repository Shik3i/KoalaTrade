# KoalaTrade 🐨📈

> Paper Trading — Wetten, Aktien, ETFs, Crypto & Leaderboards. Kein Echtgeld, nur Ruhm.

Ein Paper Trading Spiel, bei dem du mit virtuellen **$10,000** startest und gegen
Freunde im Leaderboard antrittst. Nutzt die **Polymarket CLOB API** für
Ereignis-Wetten und **Finnhub** (plus CoinGecko) für Echtzeit-Kursdaten.

---

## Tech Stack

| Komponente | Wahl |
|---|---|
| **Frontend** | Svelte 5 + Vite SPA + PWA |
| **CSS** | Tailwind CSS |
| **Icons** | Phosphor Icons |
| **Backend** | Go + Chi Router + sqlx |
| **Datenbank** | SQLite (Server), IndexedDB (Client) |
| **Auth** | JWT (stateless) |
| **Marktdaten** | Finnhub (Aktien/ETFs/Gold) + CoinGecko (Crypto) + Polymarket CLOB |
| **Hosting** | Hetzner VPS + Caddy + Docker/Dockge |

## Features

### MVP
- [x] **Virtuelles Depot** — $10,000 Startguthaben, Portfolio-Übersicht
- [x] **Polymarket-Wetten** — Yes/No-Anteile kaufen/verkaufen via CLOB API-Preise
- [x] **Wertpapiere** — Aktien, ETFs, Gold, Crypto über Finnhub + CoinGecko
- [x] **Echtzeit-Kurse** — Server-seitiger Price-Cache (alle 1-2 Min aktualisiert)
- [x] **Leaderboard** — Total Worth + % Growth (Tag/Woche/Monat/Jahr)
- [x] **Local-First** — IndexedDB, offline nutzbar
- [x] **Optionaler Account** — Nutzername + Passwort für Sync & Leaderboard
- [x] **PWA** — Installierbar als App (Service Worker + Manifest)
- [x] **Dark Theme** — Default

### Später / Nice-to-Have
- [ ] **Seasons** — Freiwilliger 3-Monats-Reset mit Startguthaben
- [ ] **Private Gruppen-Leaderboards**
- [ ] **Leerverkäufe** (vorbereitet, erstmal deaktiviert)
- [ ] **Order-Typen** — Limit/Stop-Loss (simuliert)
- [ ] **Watchlist**
- [ ] **Statistiken & Badges**
- [ ] **Eigene API für Drittanbieter**

## Architektur

### Lokal-First + Sync

```
Browser (IndexedDB)
├── portfolio        — Aktuelle Positionen
├── transactions     — Trade-Historie
├── watchlist        — Gemerkte Märkte
└── user_profile     — Nutzername (wenn registriert)
        │
        ▼  Sync bei Registration
Server (Go + SQLite)
├── users            — Nutzername + bcrypt-Hash
├── transactions     — Kopie der Trades
├── leaderboard      — Depotwerte + Growth-Raten
└── price_cache      — Gecachte Kursdaten
```

### Price-Update-Strategie

Server pollt Preise im Hintergrund (Goroutine + Ticker):

| Asset | Quelle | Takt |
|---|---|---|
| Aktien / ETFs | Finnhub (60 calls/min) | Jedes Symbol ~alle 1-2min |
| Crypto | CoinGecko (30 calls/min, kein Key) | Alle 1min |
| Gold / Rohstoffe | Finnhub | Teil der Rotation |
| Polymarket-Märkte | CLOB API (unlimitiert) | On-Demand + Cache |

Alle Clients beziehen Preise vom Server-Cache — ein API-Call bedient 100 User.

## Projektstruktur

```
koalatrade/
├── LICENSE                  # MIT
├── README.md
├── .gitignore
├── Makefile
├── docker-compose.yml
│
├── backend/                 # Go API-Server
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── handler/         # HTTP-Routen
│   │   ├── model/           # Datenmodelle
│   │   ├── repository/      # SQLite-Zugriff
│   │   └── service/         # Business-Logik + Price-Fetcher
│   ├── go.mod
│   └── Dockerfile
│
├── frontend/                # Svelte 5 SPA + PWA
│   ├── src/
│   │   ├── routes/          # SPA-Routen
│   │   ├── lib/
│   │   │   ├── components/  # UI-Komponenten
│   │   │   └── stores/      # IndexedDB/LocalStorage
│   │   ├── app.html
│   │   └── service-worker.js
│   ├── static/
│   │   └── manifest.json
│   ├── package.json
│   ├── svelte.config.js
│   └── Dockerfile
│
└── docker-compose.yml
```

## Umgebungsvariablen

Kopiere `backend/.env.example` nach `backend/.env` und trage Keys ein:

```
FINNHUB_API_KEY=dein_key
POLYMARKET_API_KEY= # optional
```

## Entwicklung

```bash
# Backend starten
make dev-backend

# Frontend starten
make dev-frontend

# Full-Stack mit Docker
make docker-up
```

## Lizenz

MIT — see [LICENSE](LICENSE).
