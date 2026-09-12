import { flushPromises, mount } from '@vue/test-utils'
import GraphPage from './GraphPage.vue'

const structure = {
	structure_id: 'structure-1',
	community_set_id: 'communities-1',
	corpora_id: 'corpus-1',
}

const alpha = {
  id: 'entity-a', version: 3, title: 'Alpha', type: 'person', aliases: [],
	description: 'Alpha entity', degree: 4, text_unit_count: 2,
}

const beta = {
  id: 'entity-b', version: 1, title: 'Beta', type: 'org', aliases: [],
	description: '', degree: 2, text_unit_count: 1,
}

function json(value: unknown): Response {
  return new Response(JSON.stringify(value), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

describe('GraphPage', () => {
  it('loads a fixed Entity graph and progressively expands a selected neighborhood', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities/entity-a/neighbors')) {
        return Promise.resolve(json({
		  center: alpha,
          entities: [alpha, beta],
          relations: [{
			id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_relations: 1,
          truncated: false,
        }))
      }
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
		  entities: [alpha],
          relations: [],
          matched_entities: 1,
          matched_relations: 0,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

	expect(wrapper.find('.publication-context').text()).toContain('当前正式版本')
    expect(wrapper.findAll('.entity-node')).toHaveLength(1)
    await wrapper.find('.entity-node').trigger('click')
    expect(wrapper.find('.graph-inspector').text()).toContain('Alpha')
    await wrapper.find('.inspector-action').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities/entity-a/neighbors?limit=100',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(wrapper.findAll('.entity-node')).toHaveLength(2)
    expect(wrapper.findAll('.graph-edge')).toHaveLength(1)
  })

  it('selects relation from graph canvas interaction', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
		  entities: [alpha, beta],
          relations: [{
			id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_entities: 2,
          matched_relations: 1,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

    expect(wrapper.findAll('.graph-edge')).toHaveLength(1)
    await wrapper.find('.graph-edge').trigger('click')

    expect(wrapper.find('.graph-inspector').text()).toContain('RELATION · v2')
    expect(wrapper.find('.graph-inspector').text()).toContain('connects')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities?limit=100',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  it('supports relation click on hit area', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
          entities: [alpha, beta],
          relations: [{
            id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_entities: 2,
          matched_relations: 1,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

    const relationLine = wrapper.find('.graph-edge .graph-edge-hit-area')
    await relationLine.trigger('click')
    expect(wrapper.find('.graph-inspector').text()).toContain('RELATION · v2')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities?limit=100',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  it('supports node selection via pointer down/up', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
          entities: [alpha, beta],
          relations: [{
            id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_entities: 2,
          matched_relations: 1,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

    const node = wrapper.findAll('.graph-node')[0]
    await node.trigger('pointerdown', { pointerId: 10 })
    await node.trigger('pointerup', { pointerId: 10 })

    expect(wrapper.find('.graph-inspector').text()).toContain('Alpha')
  })

  it('supports relation selection via pointer up', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
          entities: [alpha, beta],
          relations: [{
            id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_entities: 2,
          matched_relations: 1,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

    const relation = wrapper.find('.graph-edge')
    await relation.trigger('pointerup', { pointerId: 11 })
    expect(wrapper.find('.graph-inspector').text()).toContain('RELATION · v2')
    expect(wrapper.find('.graph-inspector').text()).toContain('connects')
  })

  it('selects entity and relation through direct SVG hit targets', async () => {
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
          entities: [alpha, beta],
          relations: [{
            id: 'relation-ab', version: 2, source_entity_id: 'entity-a', target_entity_id: 'entity-b', type: 'connects_to',
            description: 'connects', weight: 2, combined_degree: 6, text_unit_count: 1,
          }],
          matched_entities: 2,
          matched_relations: 1,
          truncated: false,
        }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()

    await wrapper.find('.graph-node').trigger('click')
    expect(wrapper.find('.graph-inspector').text()).toContain('Alpha')

    await wrapper.find('.graph-edge .graph-edge-hit-area').trigger('click')
    expect(wrapper.find('.graph-inspector').text()).toContain('RELATION · v2')
  })

	it('loads Community roots and expands children within one Structure', async () => {
    const root = {
      id: 'community-root', number: 0, level: 0, child_count: 1, entity_count: 2,
    }
    const child = {
      id: 'community-child', number: 1, level: 1, parent_id: 'community-root', child_count: 0,
	  entity_count: 1,
    }
    const otherRoot = {
      id: 'community-other', number: 2, level: 0, child_count: 0, entity_count: 1,
    }
    const fetchMock = vi.fn().mockImplementation((request: string) => {
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/entities')) {
        return Promise.resolve(json({
		  entities: [alpha], relations: [], matched_entities: 1, matched_relations: 0, truncated: false,
        }))
      }
      if (request.includes('parent_id=community-root')) {
		return Promise.resolve(json({ ...structure, page: 1, page_size: 100, total: 1, communities: [child] }))
      }
      if (request.includes('page=2')) {
		return Promise.resolve(json({ ...structure, page: 2, page_size: 50, total: 2, communities: [otherRoot] }))
      }
      if (request.startsWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/graph/communities')) {
		return Promise.resolve(json({ ...structure, page: 1, page_size: 50, total: 2, communities: [root] }))
      }
      throw new Error(`unexpected request ${request}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(GraphPage)
    await flushPromises()
    await wrapper.findAll('.graph-mode button')[1].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.community-node')).toHaveLength(1)

    await wrapper.find('.graph-load-more').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.community-node')).toHaveLength(2)

    await wrapper.find('.community-node').trigger('click')
	expect(wrapper.find('.graph-inspector').text()).toContain('community-root')
    await wrapper.find('.graph-inspector .outline-button').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.community-node')).toHaveLength(3)
    expect(wrapper.findAll('.community-edge')).toHaveLength(1)
	expect(wrapper.find('.publication-context').text()).toContain('structure-1')
  })
})
