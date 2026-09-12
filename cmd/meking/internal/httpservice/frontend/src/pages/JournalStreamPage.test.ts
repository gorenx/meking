import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import JournalStreamPage from './JournalStreamPage.vue'

const apiMock = vi.hoisted(() => ({ journalStream: vi.fn() }))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { code: string, message: string }) {
      super(failure.message)
    }
  },
}))

describe('JournalStreamPage', () => {
  beforeEach(() => {
    apiMock.journalStream.mockReset().mockImplementation((_id: string, after: string) => Promise.resolve({
      zone_id: '10000000-0000-4000-8000-000000000001', stream_id: 'knowledge/formal',
      after_sequence: after, next_after_sequence: after === '0' ? '2' : '3', has_more: after === '0',
      events: [{
        position: after === '0' ? '8' : '10', event_id: after === '0' ? 'event-8' : 'event-10',
        type: after === '0' ? 'knowledge.published' : 'knowledge.entity_vectors_indexed',
        schema_version: 1, stream_id: 'knowledge/formal',
        stream_sequence: after === '0' ? '2' : '3', occurred_at: '2026-08-01T12:00:00Z',
        correlation_id: 'request-1', causation_id: 'event-7',
      }],
    }))
  })

  it('continues from the last returned Stream Sequence and marks the persisted tail', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/journal', component: { template: '<div />' } },
        { path: '/journal/streams/:id(.*)', component: JournalStreamPage },
      ],
    })
    await router.push('/journal/streams/knowledge/formal')
    await router.isReady()
    const wrapper = mount(JournalStreamPage, { global: { plugins: [router] } })
    await flushPromises()

    expect(apiMock.journalStream).toHaveBeenNthCalledWith(
      1, 'knowledge/formal', '0', 100, expect.any(AbortSignal),
    )
    expect(wrapper.find('.stream-tail').text()).toBe('当前已加载末尾')

    await wrapper.find('.journal-footer button').trigger('click')
    await flushPromises()

    expect(apiMock.journalStream).toHaveBeenNthCalledWith(
      2, 'knowledge/formal', '2', 100, expect.any(AbortSignal),
    )
    expect(wrapper.findAll('.stream-timeline li')).toHaveLength(2)
    expect(wrapper.find('.stream-tail').text()).toBe('最后持久化事实')
    expect(wrapper.text()).toContain('knowledge.entity_vectors_indexed')
    wrapper.unmount()
  })
})
