import type {
  ApiFailure,
  CommunityGraph,
  CommunityGraphFilter,
  ControlAction,
  ControlActionStatusList,
  ControlInvocation,
  ControlPolicy,
  DocumentCatalogPage,
  DocumentReceipt,
  EntityGraph,
  EntityGraphFilter,
  EntityNeighborhood,
  JournalEventPage,
  JournalStreamPage,
  KnowledgeClaimPage,
  KnowledgeEntityDetail,
  KnowledgeEntityPage,
  KnowledgeRelationDetail,
  KnowledgeRelationPage,
  QueryRequest,
  QueryResponse,
  PublishControlPolicy,
  QuestionSuggestionRequest,
  QuestionSuggestionResponse,
  ReportDetail,
  ReportPage,
  RuntimeInfo,
  ZoneDefinition,
  ZoneDefinitionList,
} from './types'
import { zoneAPIPath } from '../zoneSession'

export class ApiError extends Error {
  constructor(public readonly failure: ApiFailure, public readonly status: number) {
    super(failure.message)
    this.name = 'ApiError'
  }
}

async function readFailure(response: Response): Promise<ApiFailure> {
  try {
    const body = (await response.json()) as { error?: ApiFailure }
    if (body.error) return body.error
  } catch {
    // The stable fallback does not include response text, which may be an intermediary page.
  }
  return { code: 'http_failure', message: `请求失败（HTTP ${response.status}）` }
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
  if (!response.ok) throw new ApiError(await readFailure(response), response.status)
  return (await response.json()) as T
}

function jsonRequest(body: unknown, signal?: AbortSignal, method = 'POST'): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  }
}

function queryPath(path: string, values: Record<string, string | number | undefined>): string {
  const query = new URLSearchParams()
  for (const [name, value] of Object.entries(values)) {
    if (value !== undefined && value !== '') query.set(name, String(value))
  }
  const encoded = query.toString()
  return encoded ? `${path}?${encoded}` : path
}

function pathIdentity(value: string): string {
  return value.split('/').map(encodeURIComponent).join('/')
}

export const api = {
  zones: () => requestJSON<ZoneDefinitionList>('/api/v1/zones'),
  createZone: (parentZoneID?: string) => requestJSON<ZoneDefinition>(
    '/api/v1/zones',
    jsonRequest(parentZoneID ? { parent_zone_id: parentZoneID } : {}),
  ),
  runtime: () => requestJSON<RuntimeInfo>(zoneAPIPath('/runtime')),
  documents: (offset = 0, limit = 50, signal?: AbortSignal) =>
    requestJSON<DocumentCatalogPage>(
      queryPath(zoneAPIPath('/documents'), { offset, limit }),
      { signal },
    ),
  submitDocument: (file: File, signal?: AbortSignal) => {
    const body = new FormData()
    body.append('file', file)
    return requestJSON<DocumentReceipt>(zoneAPIPath('/documents'), { method: 'POST', body, signal })
  },
  controlActions: (signal?: AbortSignal) =>
    requestJSON<ControlActionStatusList>(zoneAPIPath('/control/actions'), { signal }),
  invokeControlAction: (action: ControlAction, signal?: AbortSignal) =>
    requestJSON<ControlInvocation>(
      zoneAPIPath(`/control/actions/${encodeURIComponent(action)}/invoke`),
      { method: 'POST', signal },
    ),
  publishControlPolicy: (
    action: ControlAction,
    input: PublishControlPolicy,
    signal?: AbortSignal,
  ) => requestJSON<ControlPolicy>(
    `/api/v1/control/policies/${encodeURIComponent(action)}`,
    jsonRequest(input, signal, 'PUT'),
  ),
  journalEvents: (offset = '0', limit = 50, signal?: AbortSignal) =>
    requestJSON<JournalEventPage>(
      queryPath(zoneAPIPath('/journal/events'), { offset, limit }),
      { signal },
    ),
  journalStream: (id: string, afterSequence = '0', limit = 50, signal?: AbortSignal) =>
    requestJSON<JournalStreamPage>(
      queryPath(zoneAPIPath(`/journal/streams/${pathIdentity(id)}`), {
        after_sequence: afterSequence, limit,
      }),
      { signal },
    ),
  reports: (page = 1, pageSize = 20) =>
    requestJSON<ReportPage>(zoneAPIPath(`/community-reports?page=${page}&page_size=${pageSize}`)),
  report: (id: string, epochID: number) => requestJSON<ReportDetail>(
    queryPath(zoneAPIPath(`/community-reports/${encodeURIComponent(id)}`), { epoch_id: epochID }),
  ),
  entityGraph: (filter: EntityGraphFilter = {}, signal?: AbortSignal) =>
    requestJSON<EntityGraph>(queryPath(zoneAPIPath('/graph/entities'), { ...filter }), { signal }),
  entityNeighborhood: (id: string, limit = 50, signal?: AbortSignal) =>
    requestJSON<EntityNeighborhood>(
      queryPath(zoneAPIPath(`/graph/entities/${encodeURIComponent(id)}/neighbors`), { limit }),
      { signal },
    ),
  communityGraph: (filter: CommunityGraphFilter = {}, signal?: AbortSignal) =>
    requestJSON<CommunityGraph>(queryPath(zoneAPIPath('/graph/communities'), { ...filter }), { signal }),
  knowledgeEntities: (after = '', limit = 50, signal?: AbortSignal) =>
    requestJSON<KnowledgeEntityPage>(
      queryPath(zoneAPIPath('/knowledge/entities'), { after, limit }),
      { signal },
    ),
  knowledgeEntity: (id: string, epochID: number, signal?: AbortSignal) =>
    requestJSON<KnowledgeEntityDetail>(
      queryPath(zoneAPIPath(`/knowledge/entities/${encodeURIComponent(id)}`), { epoch_id: epochID }),
      { signal },
    ),
  knowledgeRelations: (after = '', limit = 50, signal?: AbortSignal) =>
    requestJSON<KnowledgeRelationPage>(
      queryPath(zoneAPIPath('/knowledge/relations'), { after, limit }),
      { signal },
    ),
  knowledgeRelation: (id: string, epochID: number, signal?: AbortSignal) =>
    requestJSON<KnowledgeRelationDetail>(
      queryPath(zoneAPIPath(`/knowledge/relations/${encodeURIComponent(id)}`), { epoch_id: epochID }),
      { signal },
    ),
  knowledgeClaims: (after = '', limit = 50, signal?: AbortSignal) =>
    requestJSON<KnowledgeClaimPage>(
      queryPath(zoneAPIPath('/knowledge/claims'), { after, limit }),
      { signal },
    ),
  query: (input: QueryRequest, signal?: AbortSignal) =>
    requestJSON<QueryResponse>(zoneAPIPath('/query'), jsonRequest(input, signal)),
  suggestQuestions: (input: QuestionSuggestionRequest, signal?: AbortSignal) =>
    requestJSON<QuestionSuggestionResponse>(zoneAPIPath('/question-suggestions'), jsonRequest(input, signal)),
}

export interface QueryStreamHandlers {
  delta(text: string): void
  final(result: QueryResponse): void
  error(failure: ApiFailure): void
  ping?(): void
}

interface ServerEvent {
  event: string
  data: string
}

function parseEvent(block: string): ServerEvent | null {
  let event = ''
  const data: string[] = []
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  return event && data.length ? { event, data: data.join('\n') } : null
}

function deliverEvent(message: ServerEvent, handlers: QueryStreamHandlers): boolean {
  switch (message.event) {
    case 'delta':
      handlers.delta((JSON.parse(message.data) as { text: string }).text)
      return false
    case 'final':
      handlers.final(JSON.parse(message.data) as QueryResponse)
      return true
    case 'error':
      handlers.error(JSON.parse(message.data) as ApiFailure)
      return true
    case 'ping':
      handlers.ping?.()
      return false
    default:
      throw new Error(`未知的查询流事件：${message.event}`)
  }
}

export async function streamQuery(
  input: QueryRequest,
  handlers: QueryStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const response = await fetch(zoneAPIPath('/query/stream'), jsonRequest(input, signal))
  if (!response.ok) throw new ApiError(await readFailure(response), response.status)
  if (!response.body) throw new Error('浏览器未提供查询响应流')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let terminal = false
  while (true) {
    const { value, done } = await reader.read()
    buffer += decoder.decode(value, { stream: !done }).replaceAll('\r\n', '\n')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      const message = parseEvent(buffer.slice(0, boundary))
      buffer = buffer.slice(boundary + 2)
      if (message) terminal = deliverEvent(message, handlers) || terminal
      boundary = buffer.indexOf('\n\n')
    }
    if (done) break
  }
  if (!terminal) throw new Error('查询响应流在终态事件之前结束')
}
