import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ReportsPage from './ReportsPage.vue'

const apiMock = vi.hoisted(() => ({
  reports: vi.fn(),
  report: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { message: string }) {
      super(failure.message)
    }
  },
}))

describe('ReportsPage', () => {
  beforeEach(() => {
    apiMock.reports.mockReset().mockImplementation((page = 1, pageSize = 10) => Promise.resolve({
      epoch_id: 9,
      report_set_id: 'report-set-1', community_set_id: 'community-set-1',
      corpora_id: 'corpus-set-1', page, page_size: pageSize, total: 11,
      reports: [{
        id: `report-${page}`, community_id: `community-${page}`, community_number: page - 1, level: 0,
        title: `Community report ${page}`, summary: 'Summary', rank: 6.5, period: '', size: 1,
      }],
    }))
    apiMock.report.mockReset().mockResolvedValue({
      epoch_id: 9,
      report_set_id: 'report-set-1', community_set_id: 'community-set-1',
      corpora_id: 'corpus-set-1', id: 'report-1',
      community_id: 'community-1', community_number: 0, level: 0, children: [], title: 'Community report',
      summary: 'Summary', full_content: 'Full report', rating_explanation: '', findings: [],
      rank: 6.5, period: '', size: 1,
      sources: {
        entities: [{ id: 'entity-1', version: 3 }],
        relations: [{ id: 'relation-1', version: 2 }],
        claims: [{ id: 'claim-1', version: 1, evidence_index: 0 }],
        text_unit_ids: ['unit-1'],
      },
    })
  })

  it('opens a report selected from the index', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/reports/:id?', name: 'reports', component: ReportsPage }],
    })
    await router.push('/reports')
    await router.isReady()
    const wrapper = mount(ReportsPage, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.find('.report-index > button').trigger('click')
    await flushPromises()

    expect(apiMock.report).toHaveBeenCalledWith('report-1', 9)
    expect(wrapper.find('.report-detail').text()).toContain('Full report')
    expect(wrapper.find('.source-closure').text()).toContain('entity-1')
    expect(wrapper.find('.source-closure').text()).toContain('relation-1')
    expect(wrapper.find('.source-closure').text()).toContain('claim-1')
    expect(wrapper.find('.source-closure').text()).toContain('unit-1')
    expect(wrapper.find('.report-source-grid a').attributes('href')).toContain('epoch_id=9')
    wrapper.unmount()
  })

  it('pages the current ReportSet with the API page size', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/reports/:id?', name: 'reports', component: ReportsPage }],
    })
    await router.push('/reports')
    await router.isReady()
    const wrapper = mount(ReportsPage, { global: { plugins: [router] } })
    await flushPromises()

    const next = wrapper.findAll('.report-pagination button').find((button) => button.text() === '下一页')
    await next?.trigger('click')
    await flushPromises()

    expect(apiMock.reports).toHaveBeenNthCalledWith(1, 1, 10)
    expect(apiMock.reports).toHaveBeenNthCalledWith(2, 2, 10)
    expect(wrapper.find('.report-index').text()).toContain('Community report 2')
    expect(wrapper.find('.report-pagination').text()).toContain('第 2 页')
    wrapper.unmount()
  })
})
