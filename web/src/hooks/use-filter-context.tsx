import { createContext, useContext, useMemo, useRef, useState, type ReactNode } from "react"
import type { DisplayMode, DNSModeFilter, TimeRangePreset } from "@/lib/types"

const RANGE_KEY = "mmonitor:range-preset"
const MODE_KEY = "mmonitor:display-mode"
const DNS_KEY = "mmonitor:dns-filter"
const BACKEND_KEY = "mmonitor:backend-id"

interface FilterContextValue {
  backendId: number
  setBackendId: (id: number) => void
  preset: TimeRangePreset
  setPreset: (preset: TimeRangePreset) => void
  mode: DisplayMode
  setMode: (mode: DisplayMode) => void
  dnsMode: DNSModeFilter
  setDNSMode: (dnsMode: DNSModeFilter) => void
  customStart: number
  customEnd: number
  setCustomRange: (start: number, end: number) => void
  start: number
  end: number
  bucketMs: number
  refreshNonce: number
  triggerRefresh: () => void
}

const FilterContext = createContext<FilterContextValue | null>(null)

function computePresetBounds(preset: TimeRangePreset, now: Date) {
  const end = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59, 999).getTime()
  if (preset === "all") {
    return { start: 0, end }
  }
  const daysBackMap: Record<Exclude<TimeRangePreset, "custom" | "all">, number> = {
    today: 0,
    "1d": 1,
    "3d": 3,
    "7d": 7,
    "15d": 15,
    "30d": 30,
  }
  const daysBack = preset === "custom" ? 7 : daysBackMap[preset]
  const start = new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate() - daysBack,
    0,
    0,
    0,
    0,
  ).getTime()
  return { start, end }
}

function computeBucketMs(start: number, end: number, mode: DisplayMode): number {
  if (start <= 0) return 86400000
  const span = Math.max(0, end - start)
  let size = 60000
  if (span <= 3600000) size = 60000
  else if (span <= 86400000) size = 300000
  else if (span <= 604800000) size = 3600000
  else size = 86400000
  return mode === "summary" ? Math.max(86400000, size) : size
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [backendId, setBackendIdState] = useState<number>(() => {
    try {
      const saved = Number(window.localStorage.getItem(BACKEND_KEY))
      if (Number.isInteger(saved) && saved >= 0) return saved
    } catch {
      // ignore
    }
    return 0
  })

  const [mode, setModeState] = useState<DisplayMode>(() => {
    try {
      const v = window.localStorage.getItem(MODE_KEY)
      if (v === "detail" || v === "summary") return v
    } catch {
      // ignore
    }
    return "detail"
  })

  const [preset, setPresetState] = useState<TimeRangePreset>(() => {
    try {
      const v = window.localStorage.getItem(RANGE_KEY) as TimeRangePreset | null
      if (v && ["today", "1d", "3d", "7d", "15d", "30d", "all"].includes(v)) {
        if (v === "all" && mode === "detail") return "7d"
        return v
      }
    } catch {
      // ignore
    }
    return "7d"
  })

  const lastDetailPresetRef = useRef<TimeRangePreset>(preset === "all" ? "7d" : preset)

  const [dnsMode, setDNSModeState] = useState<DNSModeFilter>(() => {
    try {
      const v = window.localStorage.getItem(DNS_KEY) as DNSModeFilter | null
      if (v && ["all", "fake-ip", "fake-ip-unresolved", "normal", "redir-host"].includes(v)) {
        return v
      }
    } catch {
      // ignore
    }
    return "all"
  })

  const initialBounds = computePresetBounds("7d", new Date())
  const [customStart, setCustomStart] = useState(initialBounds.start)
  const [customEnd, setCustomEnd] = useState(initialBounds.end)
  const [refreshNonce, setRefreshNonce] = useState(0)

  const setBackendId = (id: number) => {
    const safe = Number.isInteger(id) && id >= 0 ? id : 0
    setBackendIdState(safe)
    try {
      window.localStorage.setItem(BACKEND_KEY, String(safe))
    } catch {
      // ignore
    }
  }

  const setPreset = (next: TimeRangePreset) => {
    setPresetState(next)
    if (next !== "all") {
      lastDetailPresetRef.current = next
    }
    if (next !== "custom") {
      try {
        window.localStorage.setItem(RANGE_KEY, next)
      } catch {
        // ignore
      }
    }
  }

  // Switching Detail <-> Summary preserves the user's current time range.
  // Only if "all" (summary-only) is active and switching back to Detail,
  // restore the previous non-all preset (default 7d) instead of custom.
  const setMode = (next: DisplayMode) => {
    setModeState(next)
    if (next === "detail" && preset === "all") {
      const fallback = lastDetailPresetRef.current === "all" ? "7d" : lastDetailPresetRef.current
      setPresetState(fallback)
    }
    try {
      window.localStorage.setItem(MODE_KEY, next)
    } catch {
      // ignore
    }
  }

  const setDNSMode = (next: DNSModeFilter) => {
    setDNSModeState(next)
    try {
      window.localStorage.setItem(DNS_KEY, next)
    } catch {
      // ignore
    }
  }

  const setCustomRange = (s: number, e: number) => {
    setCustomStart(s)
    setCustomEnd(e)
    setPresetState("custom")
    lastDetailPresetRef.current = "custom"
  }

  const triggerRefresh = () => setRefreshNonce((n) => n + 1)

  const { start, end, bucketMs } = useMemo(() => {
    void refreshNonce
    if (preset === "custom") {
      const s = Math.min(customStart, customEnd)
      const e = Math.max(customStart, customEnd)
      return { start: s, end: e, bucketMs: computeBucketMs(s, e, mode) }
    }
    const bounds = computePresetBounds(preset, new Date())
    return {
      start: bounds.start,
      end: bounds.end,
      bucketMs: computeBucketMs(bounds.start, bounds.end, mode),
    }
  }, [preset, customStart, customEnd, mode, refreshNonce])

  return (
    <FilterContext.Provider
      value={{
        backendId,
        setBackendId,
        preset,
        setPreset,
        mode,
        setMode,
        dnsMode,
        setDNSMode,
        customStart,
        customEnd,
        setCustomRange,
        start,
        end,
        bucketMs,
        refreshNonce,
        triggerRefresh,
      }}
    >
      {children}
    </FilterContext.Provider>
  )
}

export function useFilters() {
  const ctx = useContext(FilterContext)
  if (!ctx) throw new Error("useFilters must be used inside FilterProvider")
  return ctx
}
