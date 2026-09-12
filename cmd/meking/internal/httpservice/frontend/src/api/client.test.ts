import { api, streamQuery } from './client'
import type { QueryResponse } from './types'
import { selectZone } from '../zoneSession'

const zoneID = '10000000-0000-4000-8000-000000000001'

beforeEach(() => selectZone(zoneID))

function streamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  return new Response(new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      controller.close()
    },
  }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

describe('streamQuery', () => {
  it('parses chunk boundaries and requires one terminal event', async () => {
    const result = {
	  method: 'local', epoch_id: 7, corpora_id: 'corpus-1',
	  response: 'Hello',
	  citation_audit: { missing: true, items: [] },
	} satisfies QueryResponse
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamResponse([
      'event: delta\ndata: {"text":"Hel',
      'lo"}\n\nevent: ping\ndata: {}\n\n',
      `event: final\ndata: ${JSON.stringify(result)}\n\n`,
    ])))
    const deltas: string[] = []
    let terminalEpoch = 0

    await streamQuery({ method: 'local', question: 'Question' }, {
      delta: (text) => deltas.push(text),
      final: (value) => { terminalEpoch = value.epoch_id ?? 0 },
      error: () => { throw new Error('unexpected error event') },
    })

    expect(deltas).toEqual(['Hello'])
    expect(terminalEpoch).toBe(7)
  })

  it('rejects a stream that closes without final or error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(streamResponse([
      'event: delta\ndata: {"text":"partial"}\n\n',
    ])))

    await expect(streamQuery({ method: 'local', question: 'Question' }, {
      delta: () => undefined,
      final: () => undefined,
      error: () => undefined,
    })).rejects.toThrow('终态事件')
  })
})

describe('api.submitDocument', () => {
  it('sends exactly one multipart file without setting a manual content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      zone_id: 'zone-1', document_id: 'document-1',
      content_digest: 'abc123', status: 'uploaded',
    }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    const file = new File(['hello'], 'notes.md', { type: 'text/markdown' })

    const receipt = await api.submitDocument(file)

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(fetchMock.mock.calls[0][0]).toBe(`/api/v1/zones/${zoneID}/documents`)
    expect(init.method).toBe('POST')
    expect(init.headers).toBeUndefined()
    expect(init.body).toBeInstanceOf(FormData)
    expect((init.body as FormData).get('file')).toBe(file)
    expect(receipt.status).toBe('uploaded')
  })
})

describe('Knowledge browse client', () => {
  it('encodes the provider cursor and page limit', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({
      epoch_id: 9, corpora_id: 'corpus-1', report_set_id: 'reports-1',
      items: [], next_after: '', has_more: false,
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
    vi.stubGlobal('fetch', fetchMock)

    await api.knowledgeEntities('entity/a b', 25)
    await api.knowledgeEntity('entity/a b', 9)
    await api.knowledgeRelations('relation/a b', 15)
    await api.knowledgeRelation('relation/a b', 9)
    await api.knowledgeClaims('claim/a b', 10)

    expect(fetchMock.mock.calls[0][0]).toBe(`/api/v1/zones/${zoneID}/knowledge/entities?after=entity%2Fa+b&limit=25`)
    expect(fetchMock.mock.calls[1][0]).toBe(`/api/v1/zones/${zoneID}/knowledge/entities/entity%2Fa%20b?epoch_id=9`)
    expect(fetchMock.mock.calls[2][0]).toBe(`/api/v1/zones/${zoneID}/knowledge/relations?after=relation%2Fa+b&limit=15`)
    expect(fetchMock.mock.calls[3][0]).toBe(`/api/v1/zones/${zoneID}/knowledge/relations/relation%2Fa%20b?epoch_id=9`)
    expect(fetchMock.mock.calls[4][0]).toBe(`/api/v1/zones/${zoneID}/knowledge/claims?after=claim%2Fa+b&limit=10`)
  })
})

describe('Report browse client', () => {
  it('keeps report detail on the list Epoch', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.report('report/a b', 9)

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/zones/${zoneID}/community-reports/report%2Fa%20b?epoch_id=9`,
      undefined,
    )
  })
})

describe('Journal browse client', () => {
  it('sends the lossless page Offset and limit', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      zone_id: zoneID, offset: '9007199254740993', next_offset: '9007199254740993',
      has_more: false, events: [],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await api.journalEvents('9007199254740993', 25)

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/zones/${zoneID}/journal/events?offset=9007199254740993&limit=25`,
      { signal: undefined },
    )
  })

  it('preserves the slash-delimited Stream identity and Sequence cursor', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      zone_id: zoneID, stream_id: 'knowledge/request 1',
      after_sequence: '9007199254740993', next_after_sequence: '9007199254740993',
      has_more: false, events: [],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await api.journalStream('knowledge/request 1', '9007199254740993', 25)

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/zones/${zoneID}/journal/streams/knowledge/request%201?after_sequence=9007199254740993&limit=25`,
      { signal: undefined },
    )
  })
})

describe('Zone document client', () => {
  it('creates a Zone and reads its Documents', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        id: zoneID, role: 'root', created_at: '2026-08-04T00:00:00Z',
      }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        zone_id: zoneID, offset: 0, has_more: false, documents: [],
      }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await api.createZone()
    await api.documents(0, 100)

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/zones')
    expect(fetchMock.mock.calls[1][0]).toBe(`/api/v1/zones/${zoneID}/documents?offset=0&limit=100`)
  })
})

describe('Control Action client', () => {
  it('reads statuses in the selected Zone', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ actions: [] }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.controlActions()

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/zones/${zoneID}/control/actions`,
      { signal: undefined },
    )
  })

  it('invokes one Action without a request body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      action: 'extract_knowledge',
    }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await api.invokeControlAction('extract_knowledge')

    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/zones/${zoneID}/control/actions/extract_knowledge/invoke`,
      { method: 'POST', signal: undefined },
    )
  })

  it('publishes a Project Policy with its expected revision', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      action: 'extract_knowledge', mode: 'automatic', minimum_pending: 3,
      maximum_wait: '5m0s', revision: 3, updated_at: '2026-09-01T01:00:00Z',
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await api.publishControlPolicy('extract_knowledge', {
      mode: 'automatic', minimum_pending: 3, maximum_wait: '5m', expected_revision: 2,
    })

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/control/policies/extract_knowledge')
    expect(init.method).toBe('PUT')
    expect(JSON.parse(String(init.body))).toEqual({
      mode: 'automatic', minimum_pending: 3, maximum_wait: '5m', expected_revision: 2,
    })
  })
})
