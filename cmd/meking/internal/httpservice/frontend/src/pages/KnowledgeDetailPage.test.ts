import { flushPromises, mount } from '@vue/test-utils'
import KnowledgeDetailPage from './KnowledgeDetailPage.vue'

const apiMock = vi.hoisted(() => ({ knowledgeEntity: vi.fn(), knowledgeRelation: vi.fn() }))

vi.mock('../api/client', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(public failure: { message: string }) {
      super(failure.message)
    }
  },
}))

const RouterLink = {
  props: ['to'],
  template: '<a><slot /></a>',
}

describe('KnowledgeDetailPage', () => {
  beforeEach(() => {
    apiMock.knowledgeEntity.mockReset().mockResolvedValue({
      epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
      entity: {
        id: 'entity-1', version: 2, title: 'Alpha', type: 'person', aliases: [],
        description: 'Description', degree: 4, evidence_count: 1,
      },
      relations: [{
        id: 'relation-1', version: 3, source_entity_id: 'entity-1', target_entity_id: 'entity-2',
        description: 'connects', weight: 2.5, combined_degree: 8, evidence_count: 1,
      }],
      neighbors: [{
        id: 'entity-2', version: 1, title: 'Beta', type: 'place', aliases: [],
        description: '', degree: 2, evidence_count: 1,
      }],
      claims: [],
      text_units: [{
        corpora_id: 'corpus-1', text_unit_id: 'unit-1', text: 'Exact evidence text',
        text_id: 'text-1', document_id: 'document-1', document_location: 'source.md', text_title: 'Source',
      }],
    })
    apiMock.knowledgeRelation.mockReset().mockResolvedValue({
      epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
      relation: {
        id: 'relation-1', version: 3, source_entity_id: 'entity-1', target_entity_id: 'entity-2',
        description: 'connects', weight: 2.5, combined_degree: 8, evidence_count: 1,
      },
      endpoints: [{
        id: 'entity-1', version: 2, title: 'Alpha', type: 'person', aliases: [],
        description: '', degree: 4, evidence_count: 1,
      }],
      claims: [], text_units: [],
    })
  })

  it('shows Entity associations and exact TextUnit evidence, then reloads a Relation', async () => {
    const wrapper = mount(KnowledgeDetailPage, {
      props: { kind: 'entity', id: 'entity-1', epochId: 9 },
      global: { stubs: { RouterLink } },
    })
    await flushPromises()

    expect(apiMock.knowledgeEntity).toHaveBeenCalledWith('entity-1', 9, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Alpha')
    expect(wrapper.text()).toContain('relation-1')
    expect(wrapper.text()).toContain('Beta')
    expect(wrapper.text()).toContain('Exact evidence text')

    await wrapper.setProps({ kind: 'relation', id: 'relation-1', epochId: 9 })
    await flushPromises()

    expect(apiMock.knowledgeRelation).toHaveBeenCalledWith('relation-1', 9, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('entity-1')
    expect(wrapper.text()).toContain('entity-2')
    wrapper.unmount()
  })
})
