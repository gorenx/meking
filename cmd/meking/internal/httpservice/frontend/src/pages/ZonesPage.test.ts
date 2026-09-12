import { flushPromises, mount } from '@vue/test-utils'
import ZonesPage from './ZonesPage.vue'

const mocks = vi.hoisted(() => ({
  zones: vi.fn(),
  createZone: vi.fn(),
  push: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: { zones: mocks.zones, createZone: mocks.createZone },
  ApiError: class ApiError extends Error {
    constructor(public failure: { code: string, message: string }) {
      super(failure.message)
    }
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push }),
}))

describe('ZonesPage', () => {
  const root = {
    id: '10000000-0000-4000-8000-000000000001', role: 'root' as const,
    created_at: '2026-08-04T00:00:00Z',
  }

  beforeEach(() => {
    localStorage.clear()
    mocks.push.mockReset().mockResolvedValue(undefined)
    mocks.zones.mockReset().mockResolvedValue({ zones: [root] })
    mocks.createZone.mockReset().mockResolvedValue({
      id: '20000000-0000-4000-8000-000000000002', role: 'child',
      parent_zone_id: root.id, created_at: '2026-08-04T00:01:00Z',
    })
  })

  it('lists Root Zones and creates a Child under the selected Root', async () => {
    const wrapper = mount(ZonesPage)
    await flushPromises()
    expect(wrapper.find('.zone-catalog').text()).toContain(root.id)

    await wrapper.find('select').setValue(root.id)
    const buttons = wrapper.findAll('button')
    const childButton = buttons.find((button) => button.text().includes('创建 Child Zone'))
    expect(childButton).toBeDefined()
    await childButton!.trigger('click')
    await flushPromises()

    expect(mocks.createZone).toHaveBeenCalledWith(root.id)
    expect(localStorage.getItem('meking.active-zone')).toBe('20000000-0000-4000-8000-000000000002')
    expect(mocks.push).toHaveBeenCalledWith('/documents')
  })
})
