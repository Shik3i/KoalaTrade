# Security Policy

## Supported versions

KoalaTrade is in early MVP development. Security fixes target the latest release
and `main`.

| Version | Supported |
|---|---|
| 0.1.x | ✅ |
| < 0.1 | ❌ |

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

- Preferred: open a [private security advisory](https://github.com/Shik3i/KoalaTrade/security/advisories/new) on GitHub.
- Include: affected component, reproduction steps, impact, and any suggested fix.

We aim to acknowledge reports within a few days and to keep you updated on the fix.

## Scope & notes

- KoalaTrade uses **virtual money only** — there are no real funds or payments.
- The server proxies all third-party market/odds APIs so provider keys never reach the browser.
- The admin area is gated by a seeded admin user and signed bearer tokens. For deployments, always set a strong `ADMIN_PASSWORD` and a fixed `AUTH_SECRET`, and serve over HTTPS.
- Never commit secrets. `.env` is git-ignored.
