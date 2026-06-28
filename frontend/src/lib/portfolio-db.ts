import { createInitialPortfolio, PORTFOLIO_ID, type PortfolioSnapshot } from './portfolio';

const DB_NAME = 'koalatrade';
const DB_VERSION = 2;
const PORTFOLIO_STORE = 'portfolios';

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

export async function savePortfolio(snapshot: PortfolioSnapshot): Promise<void> {
  const db = await openDatabase();
  await writePortfolio(db, { ...snapshot, updatedAt: new Date().toISOString() });
  db.close();
}

export async function resetPortfolio(startingCashCents: number): Promise<PortfolioSnapshot> {
  const db = await openDatabase();
  const snapshot = createInitialPortfolio(startingCashCents);
  await writePortfolio(db, snapshot);
  db.close();
  return snapshot;
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);

    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(PORTFOLIO_STORE)) {
        db.createObjectStore(PORTFOLIO_STORE, { keyPath: 'id' });
      }
    };

    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error('Unable to open portfolio database'));
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
