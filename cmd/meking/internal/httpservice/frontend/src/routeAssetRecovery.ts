const reloadAttemptKey = 'meking.route-asset-reload-at'
const reloadAttemptWindow = 10_000

export function installRouteAssetRecovery(
  events: Pick<Window, 'addEventListener'>,
  storage: Pick<Storage, 'getItem' | 'setItem'>,
  reload: () => void,
  now: () => number = Date.now,
) {
  events.addEventListener('vite:preloadError', (event) => {
    event.preventDefault()

    const occurredAt = now()
    let previousAttempt = 0
    try {
      previousAttempt = Number(storage.getItem(reloadAttemptKey)) || 0
      if (occurredAt - previousAttempt < reloadAttemptWindow) return
      storage.setItem(reloadAttemptKey, String(occurredAt))
    } catch {
      return
    }
    reload()
  })
}
