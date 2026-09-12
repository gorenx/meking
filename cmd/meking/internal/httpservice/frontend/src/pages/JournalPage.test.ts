import { flushPromises, mount } from '@vue/test-utils'
import JournalPage from './JournalPage.vue'

const apiMock = vi.hoisted(() => ({ journalEvents: vi.fn() }))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { code: string, message: string }) {
      super(failure.message)
    }
  },
}))

describe('JournalPage', () => {
  beforeEach(() => {
    apiMock.journalEvents.mockReset().mockImplementation((offset: string) => Promise.resolve({
      zone_id: '10000000-0000-4000-8000-000000000001', offset,
      next_offset: offset === '0' ? '2' : '3', has_more: offset === '0',
      events: [{
        sequence: offset === '0' ? '2' : '3', event_id: offset === '0' ? 'event-2' : 'event-3',
        type: 'knowledge.published', schema_version: 1,
        stream_id: 'knowledge/formal', stream_sequence: offset === '0' ? '2' : '3',
        occurred_at: '2026-08-01T12:00:00Z', correlation_id: 'request-1', causation_id: 'event-1',
      }],
    }))
  })

  it('browses committed events with a page Offset', async () => {
    const wrapper = mount(JournalPage)
    await flushPromises()

    expect(apiMock.journalEvents).toHaveBeenNthCalledWith(1, '0', 50, expect.any(AbortSignal))
    expect(wrapper.find('.journal-event').text()).toContain('knowledge.published')
    expect(wrapper.find('.journal-event').text()).toContain('knowledge/formal')
    expect(wrapper.find('.journal-event').text()).toContain('event-1')
    expect(wrapper.find('.journal-event a').attributes('href')).toBe(
      '/journal/streams/knowledge/formal',
    )

    await wrapper.find('.journal-footer button').trigger('click')
    await flushPromises()

    expect(apiMock.journalEvents).toHaveBeenNthCalledWith(2, '2', 50, expect.any(AbortSignal))
    expect(wrapper.findAll('.journal-event')).toHaveLength(2)
    expect(wrapper.text()).toContain('下一页 Offset 3')
    wrapper.unmount()
  })
})
