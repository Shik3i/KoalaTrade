// Lightweight, dependency-free i18n. Fits the project's privacy-first, no-CDN,
// no-runtime-deps ethos: a persisted locale store plus a reactive `t()` that
// resolves dotted keys against per-locale catalogs and interpolates {vars}.
//
// Usage in components:  {$t('nav.markets')}  /  {$t('order.filled', { symbol })}
// Usage in plain .ts:   import { get } from 'svelte/store'; get(t)('toast.saved')
import { derived, writable } from 'svelte/store';
import { en } from './locales/en';
import { de } from './locales/de';

export type Locale = 'en' | 'de';
export const LOCALES: readonly Locale[] = ['en', 'de'] as const;

// English-name of each locale for the switcher.
export const LOCALE_LABELS: Record<Locale, string> = { en: 'English', de: 'Deutsch' };

type Catalog = Record<string, unknown>;
const catalogs: Record<Locale, Catalog> = { en, de };

const STORAGE_KEY = 'koalatrade:locale';

function isLocale(value: unknown): value is Locale {
  return value === 'en' || value === 'de';
}

// Default is English (project decision); a German browser still starts in German
// unless the user has explicitly chosen a language before.
function detectInitial(): Locale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (isLocale(stored)) return stored;
  } catch {
    /* localStorage unavailable (private mode / SSR) — fall through to detection */
  }
  const nav = typeof navigator !== 'undefined' ? navigator.language?.toLowerCase() ?? '' : '';
  return nav.startsWith('de') ? 'de' : 'en';
}

export const locale = writable<Locale>(detectInitial());

locale.subscribe((value) => {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    /* ignore persistence failures */
  }
  if (typeof document !== 'undefined') document.documentElement.lang = value;
});

export function setLocale(value: Locale): void {
  locale.set(value);
}

type Vars = Record<string, string | number>;

function lookup(catalog: Catalog, key: string): string | undefined {
  const raw = key.split('.').reduce<unknown>((node, part) => {
    if (node && typeof node === 'object') return (node as Record<string, unknown>)[part];
    return undefined;
  }, catalog);
  return typeof raw === 'string' ? raw : undefined;
}

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match
  );
}

function translate(current: Locale, key: string, vars?: Vars): string {
  // Resolve against the active locale, then English, then the key itself so a
  // missing translation degrades visibly rather than crashing.
  const hit = lookup(catalogs[current], key) ?? lookup(catalogs.en, key) ?? key;
  return interpolate(hit, vars);
}

// Reactive translator: `$t('key', vars)` re-renders when the locale changes.
export const t = derived(locale, ($locale) => (key: string, vars?: Vars) =>
  translate($locale, key, vars)
);
