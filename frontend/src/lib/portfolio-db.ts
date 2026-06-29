import { createInitialPortfolio, PORTFOLIO_ID, type PortfolioSnapshot } from './portfolio';
import { defaultPreferences, type Preferences } from './preferences';

const DB_NAME = 'koalatrade';
const DB_VERSION = 3;
const PORTFOLIO_STORE = 'portfolios';
const META_STORE = 'meta';
const CLIENT_ID_KEY = 'client-id';
const PREFERENCES_KEY = 'preferences';

export async function loadPortfolio(startingCashCents: number): Promise<PortfolioSnapshot> {
  const db = await openDatabase();
  const existing = await readPortfolio(db);

  if (existing) {
    db.close();
    return existing;
  }

  const seeded = createInitialPortfolio(startingCashCents);
  await writePortfolio(db, seeded);
  db.close();
  return seeded;
}

export async function savePortfolio(
  snapshot: PortfolioSnapshot,
  options: { touchUpdatedAt?: boolean } = {}
): Promise<void> {
  const db = await openDatabase();
  await writePortfolio(db, options.touchUpdatedAt === false ? snapshot : { ...snapshot, updatedAt: new Date().toISOString() });
  db.close();
}

export async function resetPortfolio(startingCashCents: number): Promise<PortfolioSnapshot> {
  const db = await openDatabase();
  const snapshot = createInitialPortfolio(startingCashCents);
  await writePortfolio(db, snapshot);
  db.close();
  return snapshot;
}

export async function loadClientId(): Promise<string> {
  const db = await openDatabase();
  const existing = await readMeta(db, CLIENT_ID_KEY);
  if (existing) {
    db.close();
    return existing;
  }

  const clientId = crypto.randomUUID();
  await writeMeta(db, CLIENT_ID_KEY, clientId);
  db.close();
  return clientId;
}

export async function loadPreferences(): Promise<Preferences> {
  const db = await openDatabase();
  const raw = await readMeta(db, PREFERENCES_KEY);
  db.close();
  if (!raw) return defaultPreferences();
  try {
    const parsed = JSON.parse(raw) as Partial<Preferences>;
    return {
      favoriteTeams: Array.isArray(parsed.favoriteTeams) ? parsed.favoriteTeams : [],
      esportsLeagues: Array.isArray(parsed.esportsLeagues) ? parsed.esportsLeagues : defaultPreferences().esportsLeagues
    };
  } catch {
    return defaultPreferences();
  }
}

export async function savePreferences(preferences: Preferences): Promise<void> {
  const db = await openDatabase();
  await writeMeta(db, PREFERENCES_KEY, JSON.stringify(preferences));
  db.close();
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);

    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(PORTFOLIO_STORE)) {
        db.createObjectStore(PORTFOLIO_STORE, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains(META_STORE)) {
        db.createObjectStore(META_STORE, { keyPath: 'key' });
      }
    };

    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('Unable to open portfolio database'));
  });
}

function readMeta(db: IDBDatabase, key: string): Promise<string | undefined> {
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(META_STORE, 'readonly');
    const store = transaction.objectStore(META_STORE);
    const request = store.get(key);

    request.onsuccess = () => resolve((request.result as { value?: string } | undefined)?.value);
    request.onerror = () => reject(request.error ?? new Error('Unable to read local metadata'));
  });
}

function writeMeta(db: IDBDatabase, key: string, value: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(META_STORE, 'readwrite');
    const store = transaction.objectStore(META_STORE);
    store.put({ key, value });

    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error ?? new Error('Unable to write local metadata'));
  });
}

function readPortfolio(db: IDBDatabase): Promise<PortfolioSnapshot | undefined> {
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(PORTFOLIO_STORE, 'readonly');
    const store = transaction.objectStore(PORTFOLIO_STORE);
    const request = store.get(PORTFOLIO_ID);

    request.onsuccess = () => resolve(request.result as PortfolioSnapshot | undefined);
    request.onerror = () => reject(request.error ?? new Error('Unable to read portfolio'));
  });
}

function writePortfolio(db: IDBDatabase, snapshot: PortfolioSnapshot): Promise<void> {
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(PORTFOLIO_STORE, 'readwrite');
    const store = transaction.objectStore(PORTFOLIO_STORE);
    store.put(snapshot);

    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error ?? new Error('Unable to write portfolio'));
  });
}
