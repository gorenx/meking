import { installRouteAssetRecovery } from './routeAssetRecovery'

it('reloads once when a lazy route asset belongs to an older frontend build', () => {
  const events = new EventTarget()
  const values = new Map<string, string>()
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
  }
  let reloads = 0
  let now = 20_000
  installRouteAssetRecovery(events, storage, () => { reloads += 1 }, () => now)

  const staleAsset = new Event('vite:preloadError', { cancelable: true })
  events.dispatchEvent(staleAsset)
  expect(staleAsset.defaultPrevented).toBe(true)
  expect(reloads).toBe(1)

  now += 1_000
  events.dispatchEvent(new Event('vite:preloadError', { cancelable: true }))
  expect(reloads).toBe(1)
})
