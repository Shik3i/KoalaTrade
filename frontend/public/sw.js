const CACHE_NAME = 'koalatrade-shell-v1';
const SHELL_ASSETS = ['/', '/manifest.webmanifest', '/icons/koalatrade.svg'];

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_ASSETS)));
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((names) =>
      Promise.all(names.filter((name) => name !== CACHE_NAME).map((name) => caches.delete(name)))
    )
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET' || new URL(request.url).pathname.startsWith('/api')) {
    return;
  }

  event.respondWith(caches.match(request).then((cached) => cached || fetch(request)));
});
