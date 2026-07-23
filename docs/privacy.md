# KoalaTrade - Datenschutzerklärung (Privacy Policy)

## Grundsatz

KoalaTrade ist als **privacy-first** und **local-first** Papier-Trading-Desk konzipiert. Die Anwendung dient der Simulation von Börsen- und eSports-Vorhersagemärkten ohne echtes Geld. 

Der Schutz deiner Privatsphäre steht an oberster Stelle:
- **Keine Tracking-Cookies** oder Werbenetzwerke
- **Keine Analytics** oder Telemetrie-Dienste
- **Keine externen CDNs** (alle Schriftarten, Icons und Skripte werden lokal vom eigenen Server bereitgestellt)
- **Keine Direktverbindungen** des Browsers zu Drittanbieter-APIs

---

## Datenerhebung und -verarbeitung

### 1. Lokale Speicherung im Browser (IndexedDB & LocalStorage)

Standardmäßig werden deine Daten ausschließlich lokal in deinem Browser (via IndexedDB) gespeichert. Eine Registrierung oder Angabe persönlicher Daten ist für die Nutzung des Trading-Desks **nicht** erforderlich.

Folgende Daten verbleiben lokal auf deinem Gerät:

| Datenkategorie | Inhalt & Beschreibung | Zweck |
|---|---|---|
| **Portfolio & Markt** | Virtuelles Guthaben, Kauftransaktionen, offene Orders, eSports-Positionen | Berechnung deiner Portfolio-Performance |
| **Präferenzen** | Gewählte Sprache (DE/EN), präferierte eSports-Ligen & Lieblingsteams | Personalisierung des Trading-Desks |
| **Client ID** | Zufällig generierte UUID (`crypto.randomUUID()`) | Identifikator für optionalen Portfolio-Sync |
| **Session State** | Admin-Token oder Session-Cookie (falls angemeldet) | Authentifizierung im Benutzerkonto |

### 2. Serverseitige Datenverarbeitung (Backend / SQLite)

Falls du optionale Funktionen wie den **Portfolio-Sync** oder ein **Benutzerkonto** nutzt, werden folgende Daten auf dem KoalaTrade-Server (SQLite-Datenbank) verarbeitet:

- **Opt-In Portfolio-Sync**: Wenn du den Sync aktivierst, wird ein Schnappschuss deines Portfolios an das Backend gesendet. Zur Zuordnung wird aus deiner lokalen `Client-ID` ein SHA-256 Hash gebildet. Die ursprüngliche Client-ID wird nicht in der Datenbank gespeichert.
- **Benutzerkonto (Registrierung & Login)**:
  - **Benutzername & Anzeige-Name**: Frei wählbarer Name zur Kontoführung.
  - **Passwort**: Wird ausschließlich als sicherer Hash mit **PBKDF2** (mit individuellem Salt) gespeichert. Das Klartext-Passwort wird niemals gespeichert.
  - **Session-Cookies**: Bei der Anmeldung wird ein HMAC-signiertes `HttpOnly`-Cookie gesetzt, das vor XSS-Zugriffen geschützt ist.

### 3. Rechte & Datenkontrolle (DSGVO)

Du hast jederzeit die vollständige Kontrolle über deine Daten:
- **Daten-Export (JSON)**: Im Profilbereich kannst du mit einem Klick deinen gesamten Account- und Portfolio-Datensatz herunterladen.
- **Portfolio-Daten löschen**: Du kannst synchronisierte Portfolio-Daten jederzeit vom Server entfernen.
- **Konto löschen**: Du kannst dein Konto inklusive aller zugehörigen Daten mit sofortiger Wirkung vollständig löschen.

---

## Drittanbieter & API-Proxying (Privacy-Shield)

KoalaTrade verwendet Marktdaten (Aktien, ETFs, Krypto, Rohstoffe) sowie eSports-Spielpläne und Wettquoten von Drittanbietern (z. B. CoinGecko, Finnhub, LoL Esports, Polymarket).

**Schutz durch Server-Side Proxying:**
Dein Browser tritt **niemals** direkt mit diesen Drittanbietern in Kontakt. Sämtliche Netzwerkanfragen werden zentral vom KoalaTrade-Backend ausgeführt, geclustert und im Speicher gecached.
- Drittanbieter erhalten **keine IP-Adressen** unserer Nutzer.
- Drittanbieter erhalten **keine Watchlisten, Portfolios oder Positionsdaten**.
- API-Schlüssel verbleiben geschützt auf dem Server.

---

## Keine externen CDNs oder Tracking-Skripte

Sämtliche Assets (HTML, CSS, Webassembly, Icons von `@lucide/svelte`, Schriftarten wie Inter & Outfit) sind direkt im Frontend-Bundle enthalten. Es werden keine externen Webfonts (wie Google Fonts) oder externe JavaScript-Bibliotheken von CDNs nachgeladen.

---

## Open Source & Impressum

KoalaTrade ist Open-Source-Software unter der MIT-Lizenz. Der vollständige Quellcode kann eingesehen und auditiert werden:
- GitHub Repository: [https://github.com/Shik3i/KoalaTrade](https://github.com/Shik3i/KoalaTrade)

Das rechtliche Impressum und gesetzliche Angaben gemäß TDDDG / DSGVO sind abrufbar unter:
- **Legal Notice / Impressum**: [https://koalastuff.net/legal](https://koalastuff.net/legal)

---

*Stand: Juli 2026*
