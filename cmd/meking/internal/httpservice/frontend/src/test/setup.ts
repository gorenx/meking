import { afterEach, beforeEach } from 'vitest'
import { selectZone } from '../zoneSession'

beforeEach(() => {
  selectZone('10000000-0000-4000-8000-000000000001')
})

afterEach(() => {
  vi.unstubAllGlobals()
})
