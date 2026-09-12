import { flushPromises, mount } from '@vue/test-utils'
import SearchPage from './SearchPage.vue'
import { useRuntime } from '../composables/useRuntime'

const allCapabilities = ['basic', 'local', 'global', 'drift', 'streaming', 'question_suggestions', 'community_reports']

function queryResponse(response = '回答') {
  return {
    method: 'local', epoch_id: 7, report_set_id: 'report-set-1',
	community_set_id: 'community-set-1', corpora_id: 'corpus-set-1', response,
	citation_audit: { missing: true, items: [] },
  }
}

function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status, headers: { 'Content-Type': 'application/json' },
  })
}

async function setRuntime(capabilities = allCapabilities, ready = true) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
    contract_version: 11,
    application_version: 'test',
	zone_id: '10000000-0000-4000-8000-000000000001',
    epoch_id: ready ? 7 : 0,
    report_set_id: ready ? 'report-set-1' : '',
    ready,
    capabilities,
  }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  await useRuntime().refresh()
}

describe('SearchPage wireframe', () => {
  beforeEach(async () => setRuntime())

  it('orders method selection, answer region, and query composer', () => {
    const wrapper = mount(SearchPage)
    const method = wrapper.find('.method-strip')
    const intro = wrapper.find('.query-intro')
    const composer = wrapper.find('.query-composer')

    expect(method.exists()).toBe(true)
    expect(intro.exists()).toBe(true)
    expect(composer.exists()).toBe(true)
    expect(method.element.compareDocumentPosition(intro.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(intro.element.compareDocumentPosition(composer.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(wrapper.findAll('.method-strip button')).toHaveLength(4)
  })

  it('accepts visible query text', async () => {
    const wrapper = mount(SearchPage)
    const input = wrapper.find<HTMLTextAreaElement>('.query-composer textarea')
    await input.setValue('查询当前知识图')
    expect(input.element.value).toBe('查询当前知识图')
    expect(wrapper.find<HTMLButtonElement>('.submit-button').element.disabled).toBe(false)
  })

  it('explains that an unpublished Zone must be indexed before querying', async () => {
    await setRuntime(allCapabilities, false)
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('查询当前知识图')

    const submit = wrapper.find<HTMLButtonElement>('.submit-button')
    expect(submit.element.disabled).toBe(true)
    expect(submit.text()).toBe('请先完成索引发布')
  })

  it('enables the existing query form when runtime polling observes publication', async () => {
    await setRuntime(allCapabilities, false)
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('查询新发布的知识图')

    let submit = wrapper.find<HTMLButtonElement>('.submit-button')
    expect(submit.element.disabled).toBe(true)
    expect(submit.text()).toBe('请先完成索引发布')

    await setRuntime(allCapabilities, true)
    await flushPromises()

    submit = wrapper.find<HTMLButtonElement>('.submit-button')
    expect(submit.element.disabled).toBe(false)
    expect(submit.text()).toBe('开始查询')
  })

  it('only exposes methods declared by the runtime', async () => {
    await setRuntime(['basic', 'local', 'global', 'streaming'])
    const wrapper = mount(SearchPage)

    expect(wrapper.findAll('.method-strip button').map((item) => item.text())).not.toContain('DRIFT全局引导的递归探索')
    expect(wrapper.findAll('.method-strip button')).toHaveLength(3)
  })

  it('generates fixed-context question suggestions and lets the user select one', async () => {
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('当前问题？')
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      questions: ['建议一？', '建议二？'],
      epoch_id: 7,
	  report_set_id: 'report-set-1',
	  community_set_id: 'community-set-1',
	  corpora_id: 'corpus-set-1',
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await wrapper.find('.suggest-button').trigger('click')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/question-suggestions', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ history: ['当前问题？'], count: 5 }),
    }))
    expect(wrapper.find('.suggestion-card').text()).toContain('Epoch 7')
    const candidates = wrapper.findAll('.suggestion-item')
    expect(candidates).toHaveLength(2)
    await candidates[1].trigger('click')
    expect(wrapper.find<HTMLTextAreaElement>('.query-composer textarea').element.value).toBe('建议二？')
  })

  it('sends Global-only controls only with a Global request', async () => {
    const wrapper = mount(SearchPage)
    const global = wrapper.findAll('.method-strip button').find((item) => item.text().startsWith('Global'))
    await global?.trigger('click')
    await wrapper.find<HTMLInputElement>('input[type="number"]').setValue(2)
    await wrapper.findAll<HTMLInputElement>('input[type="checkbox"]')[0].setValue(true)
    await wrapper.findAll<HTMLInputElement>('input[type="checkbox"]')[1].setValue(false)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('总结知识图')

    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      method: 'global', epoch_id: 7, report_set_id: 'report-set-1',
	  community_set_id: 'community-set-1', corpora_id: 'corpus-set-1', response: '回答',
	  citation_audit: { missing: true, items: [] },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/zones/10000000-0000-4000-8000-000000000001/query', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        method: 'global', question: '总结知识图', response_type: 'Multiple Paragraphs',
        community_level: 2, dynamic_community_selection: true,
      }),
    }))
    expect(wrapper.text()).toContain('Epoch 7')
    expect(wrapper.text()).toContain('ReportSet report-set-1')
  })

  it('renders stream deltas before the fixed-publication final event', async () => {
    const encoder = new TextEncoder()
    let streamController!: ReadableStreamDefaultController<Uint8Array>
    const fetchMock = vi.fn().mockResolvedValue(new Response(new ReadableStream({
      start(controller) { streamController = controller },
    }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('流式问题')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    streamController.enqueue(encoder.encode('event: delta\ndata: {"text":"部分回答"}\n\n'))
    await flushPromises()
    expect(wrapper.find('.answer-copy').text()).toContain('部分回答')
    expect(wrapper.find('.answer-heading').text()).toContain('正在推理')

    streamController.enqueue(encoder.encode(`event: final\ndata: ${JSON.stringify(queryResponse('完整回答'))}\n\n`))
    streamController.close()
    await flushPromises()
    expect(wrapper.find('.answer-copy').text()).toContain('完整回答')
    expect(wrapper.find('.answer-heading').text()).toContain('回答完成')
    expect(wrapper.text()).toContain('Epoch 7')
    expect(wrapper.text()).toContain('Corpus corpus-set-1')
  })

  it('retains partial stream output and identifies an error terminal as interrupted', async () => {
    const encoder = new TextEncoder()
    const response = new Response(new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode('event: delta\ndata: {"text":"已生成部分"}\n\n'))
        controller.enqueue(encoder.encode(
          'event: error\ndata: {"code":"provider_failure","message":"模型连接中断","partial_output":true}\n\n',
        ))
        controller.close()
      },
    }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('部分失败')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('.answer-copy').text()).toContain('已生成部分')
    expect(wrapper.find('.answer-heading').text()).toContain('回答中断')
    expect(wrapper.find('.failure-card').text()).toContain('模型连接中断')
    expect(wrapper.find('.failure-card').text()).toContain('不会自动重试')
  })

  it('aborts the active request and reports that the unfinished turn was not retained', async () => {
    const fetchMock = vi.fn().mockImplementation((_path: string, init?: RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
    }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SearchPage)
    await wrapper.find<HTMLTextAreaElement>('.query-composer textarea').setValue('取消这个问题')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.find('.submit-button.cancel').trigger('click')
    await flushPromises()

    expect((fetchMock.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true)
    expect(wrapper.find('.query-status-card').text()).toContain('查询已停止')
    expect(wrapper.find('.query-status-card').text()).toContain('不会加入后续会话')
    expect(wrapper.find('.failure-card').exists()).toBe(false)
  })

  it('uses the synchronous endpoint when streaming is unavailable and preserves successful Local turns in order', async () => {
    await setRuntime(['basic', 'local', 'global'])
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(queryResponse('第一答')))
      .mockResolvedValueOnce(json(queryResponse('第二答')))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SearchPage)
    expect(wrapper.text()).not.toContain('流式回答')

    const input = wrapper.find<HTMLTextAreaElement>('.query-composer textarea')
    await input.setValue('第一问')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await input.setValue('第二问')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/zones/10000000-0000-4000-8000-000000000001/query')
    expect(JSON.parse((fetchMock.mock.calls[1][1] as RequestInit).body as string)).toEqual({
      method: 'local', question: '第二问', response_type: 'Multiple Paragraphs',
      conversation: [
        { role: 'user', content: '第一问' },
        { role: 'assistant', content: '第一答' },
      ],
    })
  })

  it('does not add a failed turn to the next Local conversation', async () => {
    await setRuntime(['local'])
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json({ error: { code: 'provider_failure', message: '失败' } }, 502))
      .mockResolvedValueOnce(json(queryResponse('恢复后的回答')))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mount(SearchPage)
    const input = wrapper.find<HTMLTextAreaElement>('.query-composer textarea')

    await input.setValue('失败的问题')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await input.setValue('新问题')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(JSON.parse((fetchMock.mock.calls[1][1] as RequestInit).body as string)).toEqual({
      method: 'local', question: '新问题', response_type: 'Multiple Paragraphs',
    })
  })
})
