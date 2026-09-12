export type QueryMethod = 'basic' | 'local' | 'global' | 'drift'

export interface RuntimeInfo {
  contract_version: number
  application_version: string
  zone_id: string
  epoch_id?: number
  report_set_id?: string
  ready: boolean
  capabilities: string[]
}

export interface ApiFailure {
  code: string
  message: string
  retryable?: boolean
  provider_status_code?: number
  partial_output?: boolean
}

export interface DocumentReceipt {
  zone_id: string
  document_id: string
  content_digest: string
  status: 'uploaded'
}

export interface DocumentCatalogEntry {
  document_id: string
  name: string
  media_type: string
  size: number
  content_digest: string
  selected: boolean
}

export interface DocumentCatalogPage {
  zone_id: string
  offset: number
  next_offset?: number
  has_more: boolean
  documents: DocumentCatalogEntry[]
}

export type ControlAction =
  | 'convert_document'
  | 'create_text_units'
  | 'extract_knowledge'
  | 'index_entity_vectors'
  | 'derive_community_structure'
  | 'publish_epoch'
  | 'generate_community_reports'

export type ControlPolicyMode = 'automatic' | 'manual' | 'suspended'

export interface ControlPolicy {
  action: ControlAction
  mode: ControlPolicyMode
  minimum_pending: number
  maximum_wait: string
  revision: number
  updated_at: string
}

export interface ControlActionStatus {
  zone_id: string
  action: ControlAction
  pending_count: number
  pending_since?: string
  policy: ControlPolicy
}

export interface ControlActionStatusList {
  actions: ControlActionStatus[]
}

export interface ControlInvocation {
  action: ControlAction
}

export interface PublishControlPolicy {
  mode: ControlPolicyMode
  minimum_pending: number
  maximum_wait: string
  expected_revision: number
}

export interface ZoneDefinition {
  id: string
  role: 'root' | 'child'
  parent_zone_id?: string
  created_at: string
}

export interface ZoneDefinitionList {
  zones: ZoneDefinition[]
}

export interface JournalEvent {
  sequence: string
  event_id: string
  type: string
  schema_version: number
  stream_id: string
  stream_sequence: string
  occurred_at: string
  correlation_id?: string
  causation_id?: string
}

export interface JournalEventPage {
  zone_id: string
  offset: string
  next_offset: string
  has_more: boolean
  events: JournalEvent[]
}

export interface JournalStreamPage {
  zone_id: string
  stream_id: string
  after_sequence: string
  next_after_sequence: string
  has_more: boolean
  events: JournalEvent[]
}

export interface ConversationTurn {
  role: 'system' | 'user' | 'assistant'
  content: string
}

export interface QueryRequest {
  method: QueryMethod
  question: string
  response_type?: string
  community_level?: number
  dynamic_community_selection?: boolean
  conversation?: ConversationTurn[]
  include_entity_ids?: string[]
  exclude_entity_ids?: string[]
}

export interface CitationSource {
  corpora_id: string
  text_unit_id: string
  text: string
  document_id: string
  document_location: string
  document_title: string
}

export interface Citation {
  raw: string
  dataset?: string
  record_id?: number
  parsed: boolean
  status: string
  invalid_reason?: string
  sources: CitationSource[]
}

export interface QueryResponse {
  method: QueryMethod
  epoch_id?: number
  report_set_id?: string
  community_set_id?: string
  corpora_id?: string
  response: string
  citation_audit?: {
    missing: boolean
    items: Citation[]
  }
}

export interface QuestionSuggestionRequest {
  // History is ordered oldest to newest; the last item is the current question.
  history: string[]
  count: number
}

export interface QuestionSuggestionResponse {
  questions: string[]
  epoch_id: number
  report_set_id: string
  community_set_id: string
  corpora_id: string
}

export interface KnowledgePublication {
  epoch_id: number
  corpora_id: string
  report_set_id: string
}

export interface KnowledgeEntity {
  id: string
  version: number
  title: string
  type: string
  aliases: string[]
  description: string
  degree: number
  evidence_count: number
}

export interface KnowledgeEntityPage extends KnowledgePublication {
  items: KnowledgeEntity[]
  next_after: string
  has_more: boolean
}

export interface KnowledgeRelation {
  id: string
  version: number
  source_entity_id: string
  target_entity_id: string
  description: string
  weight: number
  combined_degree: number
  evidence_count: number
}

export interface KnowledgeRelationPage extends KnowledgePublication {
  items: KnowledgeRelation[]
  next_after: string
  has_more: boolean
}

export interface KnowledgeClaimEvidence {
  text_unit_id: string
  subject_text: string
  object_text: string
  status: string
  start_date: string
  end_date: string
  description: string
  source_text: string
}

export interface KnowledgeClaim {
  id: string
  version: number
  subject_id: string
  type: string
  evidence: KnowledgeClaimEvidence[]
}

export interface KnowledgeClaimPage extends KnowledgePublication {
  items: KnowledgeClaim[]
  next_after: string
  has_more: boolean
}

export interface KnowledgeTextUnit {
  corpora_id: string
  text_unit_id: string
  text: string
  text_id: string
  document_id: string
  document_location: string
  text_title: string
}

export interface KnowledgeEntityDetail extends KnowledgePublication {
  entity: KnowledgeEntity
  relations: KnowledgeRelation[]
  neighbors: KnowledgeEntity[]
  claims: KnowledgeClaim[]
  text_units: KnowledgeTextUnit[]
}

export interface KnowledgeRelationDetail extends KnowledgePublication {
  relation: KnowledgeRelation
  endpoints: KnowledgeEntity[]
  claims: KnowledgeClaim[]
  text_units: KnowledgeTextUnit[]
}

export interface GraphEntity {
  id: string
  version: number
  title: string
  type: string
  aliases: string[]
  description: string
  degree: number
  text_unit_count: number
}

export interface GraphRelation {
  id: string
  version: number
  source_entity_id: string
  target_entity_id: string
  type: string
  description: string
  weight: number
  combined_degree: number
  text_unit_count: number
}

export interface EntityGraph {
  entities: GraphEntity[]
  relations: GraphRelation[]
  matched_entities: number
  matched_relations: number
  truncated: boolean
}

export interface EntityNeighborhood {
  center: GraphEntity
  entities: GraphEntity[]
  relations: GraphRelation[]
  matched_relations: number
  truncated: boolean
}

export interface EntityGraphFilter {
  query?: string
  entity_type?: string
  limit?: number
}

export interface GraphCommunity {
  id: string
  number: number
  level: number
  parent_id?: string
  child_count: number
  entity_count: number
}

export interface CommunityGraph {
  structure_id: string
  community_set_id: string
  corpora_id: string
  page: number
  page_size: number
  total: number
  communities: GraphCommunity[]
}

export interface CommunityGraphFilter {
  query?: string
  parent_id?: string
  page?: number
  page_size?: number
}

export interface ReportSummary {
  id: string
  community_id: string
  community_number: number
  level: number
  title: string
  summary: string
  rank: number
  period: string
  size: number
}

export interface ReportPage {
  epoch_id: number
  report_set_id: string
  community_set_id: string
  corpora_id: string
  page: number
  page_size: number
  total: number
  reports: ReportSummary[]
}

export interface ReportFinding {
  summary: string
  explanation: string
}

export interface KnowledgeReference {
  id: string
  version: number
}

export interface ClaimReference extends KnowledgeReference {
  evidence_index: number
}

export interface ReportDetail extends ReportSummary {
  epoch_id: number
  report_set_id: string
  community_set_id: string
  corpora_id: string
  parent_id?: string
  children: string[]
  full_content: string
  rating_explanation: string
  findings: ReportFinding[]
  sources: {
    entities: KnowledgeReference[]
    relations: KnowledgeReference[]
    claims: ClaimReference[]
    text_unit_ids: string[]
  }
}
