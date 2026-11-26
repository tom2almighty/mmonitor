export type Dimension = "process" | "host" | "outbound" | "rule" | "sourceIP" | "dnsMode"

export type DisplayMode = "detail" | "summary"

export type DNSModeFilter = "all" | "fake-ip" | "fake-ip-unresolved" | "normal" | "redir-host"

export type TimeRangePreset = "today" | "1d" | "3d" | "7d" | "15d" | "30d" | "all" | "custom"

export interface LiveStatus {
  appVersion: string
  backendId: number
  backendName: string
  configured: boolean
  connected: boolean
  backendCount: number
  connectedCount: number
  transport: "ws" | "http" | "disconnected"
  mihomoVersion: string
  mihomoMode: string
  dnsEnhancedMode: string
  fakeIPRange: string
  tunEnabled: boolean
  uploadSpeed: number
  downloadSpeed: number
  uploadTotal: number
  downloadTotal: number
  memory: number
  activeConnections: number
  activeFakeIPConnections: number
  unresolvedFakeIPConnections: number
  lastUpdated: number
  lastError: string
}

export interface BackendWithStatus {
  id: number
  name: string
  url: string
  hasSecret: boolean
  enabled: boolean
  createdAt: number
  status: LiveStatus
}

export interface BackendFormPayload {
  name: string
  url: string
  secret?: string
  clearSecret?: boolean
  enabled: boolean
}

export interface TestBackendRequest {
  backendId?: number
  url?: string
  secret?: string
  clearSecret?: boolean
}

export interface TestConnectionResult {
  ok: boolean
  version: string
  mode: string
  dnsEnhancedMode: string
  fakeIPRange: string
  tunEnabled: boolean
  latencyMs: number
  error?: string
}

export interface LiveConnection {
  backendId: number
  backendName: string
  id: string
  sourceIP: string
  host: string
  sniffHost?: string
  destinationIP: string
  destinationPort?: string
  process: string
  processPath?: string
  outbound: string
  chains: string[]
  ruleGroup: string
  dnsMode: string
  network: string
  inboundType?: string
  start: string
  upload: number
  download: number
  uploadSpeed: number
  downloadSpeed: number
}

export interface LiveConnectionsResponse {
  status: LiveStatus
  connections: LiveConnection[]
}

export interface Preferences {
  domainGroupingEnabled: boolean
  filterDirectTraffic: boolean
  retentionDays: number
}

export interface StorageStats {
  dbSizeBytes: number
  walSizeBytes: number
  detailRowCount: number
  summaryRowCount: number
  oldestDetailTs: number
  oldestSummaryTs: number
  newestTs: number
}

export interface VacuumResult {
  beforeBytes: number
  afterBytes: number
}

export interface PruneResult {
  detailDeleted: number
  summaryDeleted: number
}

export interface OverviewStats {
  totalUpload: number
  totalDownload: number
  totalBytes: number
  fakeIPUpload: number
  fakeIPDownload: number
  fakeIPBytes: number
  fakeIPRatio: number
  unresolvedFakeIPBytes: number
  normalBytes: number
  processCount: number
  hostCount: number
  outboundCount: number
  sourceIPCount: number
  ruleCount: number
}

export interface AggregatedRow {
  label: string
  upload: number
  download: number
  total: number
  count: number
}

export interface TrendPoint {
  timestamp: number
  upload: number
  download: number
}

export interface ConnectionDetail {
  destinationIP: string
  sourceIP: string
  process: string
  outbound: string
  chains: string[]
  ruleGroup: string
  dnsMode: string
  network: string
  upload: number
  download: number
  total: number
  count: number
}
