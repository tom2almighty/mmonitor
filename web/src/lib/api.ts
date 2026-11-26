import type {
  AggregatedRow,
  BackendFormPayload,
  BackendWithStatus,
  ConnectionDetail,
  Dimension,
  DNSModeFilter,
  LiveConnectionsResponse,
  LiveStatus,
  OverviewStats,
  Preferences,
  PruneResult,
  StorageStats,
  TestBackendRequest,
  TestConnectionResult,
  TrendPoint,
  VacuumResult,
} from "./types"

async function request<T>(
  path: string,
  options?: {
    method?: string
    params?: Record<string, string | number | boolean | undefined>
    body?: unknown
  },
): Promise<T> {
  const url = new URL(path, window.location.origin)
  if (options?.params) {
    for (const [k, v] of Object.entries(options.params)) {
      if (v !== undefined && v !== "") {
        url.searchParams.set(k, String(v))
      }
    }
  }

  const res = await fetch(url.toString(), {
    method: options?.method ?? "GET",
    headers: options?.body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: options?.body !== undefined ? JSON.stringify(options.body) : undefined,
  })

  if (!res.ok) {
    const payload = (await res.json().catch(() => ({}))) as { error?: string }
    throw new Error(payload.error || `Request failed (${res.status})`)
  }
  return res.json() as Promise<T>
}

export const api = {
  // Backends
  listBackends: () => request<BackendWithStatus[]>("/api/backends"),

  createBackend: (payload: BackendFormPayload) =>
    request<BackendWithStatus>("/api/backends", { method: "POST", body: payload }),

  updateBackend: (id: number, payload: BackendFormPayload) =>
    request<BackendWithStatus>(`/api/backends/${id}`, { method: "PUT", body: payload }),

  deleteBackend: (id: number, deleteData = false) =>
    request<{ ok: boolean }>(`/api/backends/${id}`, {
      method: "DELETE",
      params: { deleteData: deleteData ? "1" : undefined },
    }),

  testBackend: (payload: TestBackendRequest) =>
    request<TestConnectionResult>("/api/backends/test", { method: "POST", body: payload }),

  // Live telemetry
  getStatus: (backendId?: number) =>
    request<LiveStatus>("/api/status", {
      params: { backendId: backendId && backendId > 0 ? backendId : undefined },
    }),

  getLiveConnections: (backendId?: number) =>
    request<LiveConnectionsResponse>("/api/connections/live", {
      params: { backendId: backendId && backendId > 0 ? backendId : undefined },
    }),

  closeLiveConnection: (backendId: number, id: string) =>
    request<{ ok: boolean }>(`/api/connections/live/${backendId}/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  closeAllLiveConnections: (backendId?: number) =>
    request<{ ok: boolean }>("/api/connections/live", {
      method: "DELETE",
      params: { backendId: backendId && backendId > 0 ? backendId : undefined },
    }),

  // Preferences & storage management
  getPreferences: () => request<Preferences>("/api/settings/preferences"),

  updatePreferences: (payload: Preferences) =>
    request<Preferences>("/api/settings/preferences", { method: "PUT", body: payload }),

  getStorageStats: () => request<StorageStats>("/api/storage/stats"),

  vacuumStorage: () => request<VacuumResult>("/api/storage/vacuum", { method: "POST" }),

  pruneStorage: (payload: { beforeTs: number; scope: "detail" | "all"; backendId?: number }) =>
    request<PruneResult>("/api/storage/prune", { method: "POST", body: payload }),

  clearAllTraffic: (backendId?: number) =>
    request<{ ok: boolean }>("/api/traffic/logs", {
      method: "DELETE",
      params: { backendId: backendId && backendId > 0 ? backendId : undefined },
    }),

  // Traffic analytics
  getOverview: (params: { backendId?: number; start: number; end: number; summary?: boolean }) =>
    request<OverviewStats>("/api/traffic/overview", {
      params: {
        backendId: params.backendId && params.backendId > 0 ? params.backendId : undefined,
        start: params.start,
        end: params.end,
        summary: params.summary ? "1" : undefined,
      },
    }),

  getAggregate: (params: {
    backendId?: number
    dimension: Dimension
    start: number
    end: number
    summary?: boolean
    raw?: boolean
    dnsMode?: DNSModeFilter
  }) =>
    request<AggregatedRow[]>("/api/traffic/aggregate", {
      params: {
        backendId: params.backendId && params.backendId > 0 ? params.backendId : undefined,
        dimension: params.dimension,
        start: params.start,
        end: params.end,
        summary: params.summary ? "1" : undefined,
        raw: params.raw ? "1" : undefined,
        dnsMode: params.dnsMode && params.dnsMode !== "all" ? params.dnsMode : undefined,
      },
    }),

  getSubstats: (params: {
    backendId?: number
    dimension: Dimension
    label: string
    start: number
    end: number
    summary?: boolean
    dnsMode?: DNSModeFilter
  }) =>
    request<AggregatedRow[]>("/api/traffic/substats", {
      params: {
        backendId: params.backendId && params.backendId > 0 ? params.backendId : undefined,
        dimension: params.dimension,
        label: params.label,
        start: params.start,
        end: params.end,
        summary: params.summary ? "1" : undefined,
        dnsMode: params.dnsMode && params.dnsMode !== "all" ? params.dnsMode : undefined,
      },
    }),

  getConnectionDetails: (params: {
    backendId?: number
    dimension: Dimension
    primary: string
    secondary: string
    start: number
    end: number
    summary?: boolean
    dnsMode?: DNSModeFilter
  }) =>
    request<ConnectionDetail[]>("/api/traffic/details", {
      params: {
        backendId: params.backendId && params.backendId > 0 ? params.backendId : undefined,
        dimension: params.dimension,
        primary: params.primary,
        secondary: params.secondary,
        start: params.start,
        end: params.end,
        summary: params.summary ? "1" : undefined,
        dnsMode: params.dnsMode && params.dnsMode !== "all" ? params.dnsMode : undefined,
      },
    }),

  getTrend: (params: {
    backendId?: number
    start: number
    end: number
    bucket: number
    summary?: boolean
    dnsMode?: DNSModeFilter
  }) =>
    request<TrendPoint[]>("/api/traffic/trend", {
      params: {
        backendId: params.backendId && params.backendId > 0 ? params.backendId : undefined,
        start: params.start,
        end: params.end,
        bucket: params.bucket,
        summary: params.summary ? "1" : undefined,
        dnsMode: params.dnsMode && params.dnsMode !== "all" ? params.dnsMode : undefined,
      },
    }),
}
