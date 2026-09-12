import { flushPromises, mount } from '@vue/test-utils'
import KnowledgePage from './KnowledgePage.vue'

const apiMock = vi.hoisted(() => ({
  knowledgeEntities: vi.fn(),
  knowledgeRelations: vi.fn(),
  knowledgeClaims: vi.fn(),
}))
const routerPush = vi.fn()

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { message: string }) {
      super(failure.message)
    }
  },
}))

describe('KnowledgePage', () => {
  beforeEach(() => {
    routerPush.mockReset()
    apiMock.knowledgeEntities.mockReset()
      .mockResolvedValueOnce({
        epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
        items: [{
          id: 'entity-1', version: 3, title: 'Alpha', type: 'person', aliases: [],
          description: 'Description', degree: 8, evidence_count: 2,
        }],
        next_after: 'entity-1', has_more: true,
      })
      .mockResolvedValueOnce({
        epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
        items: [], next_after: 'entity-1', has_more: false,
      })
    apiMock.knowledgeRelations.mockReset().mockResolvedValue({
      epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
      items: [{
        id: 'relation-1', version: 2, source_entity_id: 'entity-1', target_entity_id: 'entity-2',
        description: 'connects', weight: 3.5, combined_degree: 12, evidence_count: 4,
      }],
      next_after: '', has_more: false,
    })
    apiMock.knowledgeClaims.mockReset().mockResolvedValue({
      epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
      items: [{
        id: 'claim-1', version: 2, subject_id: 'entity-1', type: 'status',
        evidence: [{
          text_unit_id: 'unit-1', subject_text: 'Alpha', object_text: 'active',
          status: 'true', start_date: '', end_date: '', description: 'Supported by source',
          source_text: 'Source text',
        }],
      }],
      next_after: '', has_more: false,
    })
  })

  it('shows the fixed Entity page and follows its cursor', async () => {
    const wrapper = mount(KnowledgePage, { global: { mocks: { $router: { push: routerPush } } } })
    await flushPromises()

    expect(wrapper.text()).toContain('Epoch 9')
    expect(wrapper.text()).toContain('Alpha')
    expect(wrapper.text()).toContain('v3')
    await wrapper.find('tr[role="link"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'knowledge-entity', params: { id: 'entity-1' }, query: { epoch_id: 9 },
    })

    const next = wrapper.findAll('button').find((button) => button.text() === '下一页')
    await next?.trigger('click')
    await flushPromises()

    expect(apiMock.knowledgeEntities).toHaveBeenNthCalledWith(1, '', 25, expect.any(AbortSignal))
    expect(apiMock.knowledgeEntities).toHaveBeenNthCalledWith(2, 'entity-1', 25, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('第 2 页')
    wrapper.unmount()
  })

  it('switches to the fixed Relation page', async () => {
    const wrapper = mount(KnowledgePage, { global: { mocks: { $router: { push: routerPush } } } })
    await flushPromises()

    const relations = wrapper.findAll('button').find((button) => button.text() === 'Relation')
    await relations?.trigger('click')
    await flushPromises()

    expect(apiMock.knowledgeRelations).toHaveBeenCalledWith('', 25, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('entity-1 → entity-2')
    expect(wrapper.text()).toContain('v2')
    expect(wrapper.text()).toContain('3.5')
    await wrapper.find('tr[role="link"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({
      name: 'knowledge-relation', params: { id: 'relation-1' }, query: { epoch_id: 9 },
    })
    wrapper.unmount()
  })

  it('switches to the fixed Claim page and shows 证据 sources', async () => {
    const wrapper = mount(KnowledgePage, { global: { mocks: { $router: { push: routerPush } } } })
    await flushPromises()

    const claims = wrapper.findAll('button').find((button) => button.text() === 'Claim')
    await claims?.trigger('click')
    await flushPromises()

    expect(apiMock.knowledgeClaims).toHaveBeenCalledWith('', 25, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('claim-1')
    expect(wrapper.text()).toContain('entity-1')
    expect(wrapper.text()).toContain('Supported by source')
    expect(wrapper.text()).toContain('unit-1')
    wrapper.unmount()
  })
})
