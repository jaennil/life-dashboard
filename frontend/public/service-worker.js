const CACHE_VERSION = 'life-dashboard-v3'
const APP_SHELL_CACHE = `app-shell-${CACHE_VERSION}`
const RUNTIME_CACHE = `runtime-${CACHE_VERSION}`
const APP_SHELL_FILES = [
  '/',
  '/offline.html',
  '/manifest.webmanifest',
  '/apple-touch-icon.png',
  '/icon-192.png',
  '/icon-512.png',
  '/favicon.svg',
]

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(APP_SHELL_CACHE).then((cache) => cache.addAll(APP_SHELL_FILES))
  )
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => key !== APP_SHELL_CACHE && key !== RUNTIME_CACHE)
          .map((key) => caches.delete(key))
      )
    )
  )
  self.clients.claim()
})

self.addEventListener('fetch', (event) => {
  const request = event.request
  const url = new URL(request.url)

  if (request.method !== 'GET') return
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/')) return

  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .then((response) => response)
        .catch(async () => {
          return caches.match('/offline.html')
        })
    )
    return
  }

  const isStaticAsset = ['style', 'script', 'worker', 'image', 'font'].includes(request.destination)
  if (!isStaticAsset) return

  event.respondWith(
    caches.match(request).then(async (cachedResponse) => {
      if (cachedResponse) return cachedResponse

      const response = await fetch(request)
      const cache = await caches.open(RUNTIME_CACHE)
      cache.put(request, response.clone())
      return response
    })
  )
})

self.addEventListener('push', (event) => {
  let notification = {
    title: 'Life Dashboard',
    body: 'Фоновая обработка завершена.',
    url: '/input',
    tag: 'input-result',
  }
  try {
    notification = { ...notification, ...event.data.json() }
  } catch {
    // Keep the generic notification when a push service delivers no JSON body.
  }

  event.waitUntil(self.registration.showNotification(notification.title, {
    body: notification.body,
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    tag: notification.tag,
    data: { url: notification.url },
  }))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url || '/input', self.location.origin).href
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      const existing = clients.find((client) => client.url === target)
      if (existing) return existing.focus()
      return self.clients.openWindow(target)
    })
  )
})
