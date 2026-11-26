export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB", "PB"]
  let value = bytes
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx += 1
  }
  const decimals = idx === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2
  return `${value.toFixed(decimals)} ${units[idx]}`
}

export function formatSpeed(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`
}

export function formatPercent(ratio: number, locale?: string): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return "0%"
  return new Intl.NumberFormat(locale, {
    style: "percent",
    maximumFractionDigits: ratio >= 0.1 ? 1 : 2,
  }).format(ratio)
}

export function formatNumber(value: number, locale?: string): string {
  if (!Number.isFinite(value)) return "0"
  return new Intl.NumberFormat(locale).format(value)
}

export function formatDateTime(ts: number, locale?: string): string {
  if (!Number.isFinite(ts) || ts <= 0) return "--"
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(ts))
}

export function formatDateShort(ts: number, locale?: string): string {
  if (!Number.isFinite(ts) || ts <= 0) return "--"
  return new Intl.DateTimeFormat(locale, {
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(ts))
}

export function formatShortTime(ts: number, spanMs: number, locale?: string): string {
  if (!Number.isFinite(ts) || ts <= 0) return "--"
  const date = new Date(ts)
  if (spanMs <= 86400000) {
    return new Intl.DateTimeFormat(locale, {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(date)
  }
  if (spanMs <= 604800000) {
    return new Intl.DateTimeFormat(locale, {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(date)
  }
  return new Intl.DateTimeFormat(locale, {
    month: "2-digit",
    day: "2-digit",
  }).format(date)
}
