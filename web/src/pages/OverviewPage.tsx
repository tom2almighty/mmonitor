import { useMemo } from "react"
import { useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowRight,
  ArrowUp,
  Cpu,
  Globe,
  HardDrive,
  Radio,
  Shield,
} from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { TrafficTrendChart } from "@/components/common/TrafficTrendChart"
import { TrafficDonutChart, type DonutSlice } from "@/components/common/TrafficDonutChart"
import { TrafficBarChart, type BarDatum } from "@/components/common/TrafficBarChart"
import { api } from "@/lib/api"
import { formatBytes, formatNumber, formatPercent, formatSpeed } from "@/lib/format"
import { useFilters } from "@/hooks/use-filter-context"
import type { AggregatedRow, Dimension } from "@/lib/types"

export function OverviewPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { backendId, start, end, bucketMs, mode, dnsMode } = useFilters()
  const isSummary = mode === "summary"

  const statusQuery = useQuery({
    queryKey: ["status", backendId],
    queryFn: () => api.getStatus(backendId),
    refetchInterval: 2000,
  })

  const overviewQuery = useQuery({
    queryKey: ["overview", backendId, start, end, isSummary],
    queryFn: () => api.getOverview({ backendId, start, end, summary: isSummary }),
  })

  const trendQuery = useQuery({
    queryKey: ["trend", backendId, start, end, bucketMs, isSummary, dnsMode],
    queryFn: () =>
      api.getTrend({ backendId, start, end, bucket: bucketMs, summary: isSummary, dnsMode }),
  })

  const dnsBreakdownQuery = useQuery({
    queryKey: ["aggregate", backendId, "dnsMode", start, end, isSummary, "all"],
    queryFn: () =>
      api.getAggregate({ backendId, dimension: "dnsMode", start, end, summary: isSummary }),
  })

  const topProcessesQuery = useQuery({
    queryKey: ["aggregate", backendId, "process", start, end, isSummary, dnsMode],
    queryFn: () =>
      api.getAggregate({
        backendId,
        dimension: "process",
        start,
        end,
        summary: isSummary,
        dnsMode,
      }),
  })

  const topHostsQuery = useQuery({
    queryKey: ["aggregate", backendId, "host", start, end, isSummary, dnsMode],
    queryFn: () =>
      api.getAggregate({ backendId, dimension: "host", start, end, summary: isSummary, dnsMode }),
  })

  const topOutboundsQuery = useQuery({
    queryKey: ["aggregate", backendId, "outbound", start, end, isSummary, dnsMode],
    queryFn: () =>
      api.getAggregate({
        backendId,
        dimension: "outbound",
        start,
        end,
        summary: isSummary,
        dnsMode,
      }),
  })

  const status = statusQuery.data
  const overview = overviewQuery.data

  const jumpToExplorer = (dimension: Dimension, primary?: string) => {
    const params = new URLSearchParams({ dimension })
    if (primary) params.set("primary", primary)
    navigate(`/explorer?${params.toString()}`)
  }

  const dnsSlices: DonutSlice[] = useMemo(() => {
    const colorMap: Record<string, string> = {
      "fake-ip": "var(--info)",
      "fake-ip-unresolved": "var(--warning)",
      "redir-host": "var(--chart-3)",
      normal: "var(--chart-2)",
    }
    const labelMap: Record<string, string> = {
      "fake-ip": t("dnsMode.fake-ip"),
      "fake-ip-unresolved": t("dnsMode.fake-ip-unresolved"),
      "redir-host": t("dnsMode.redir-host"),
      normal: t("dnsMode.normal"),
    }
    return (dnsBreakdownQuery.data ?? []).map((row) => ({
      key: row.label,
      name: labelMap[row.label] ?? row.label,
      value: row.total,
      color: colorMap[row.label] ?? "var(--chart-5)",
    }))
  }, [dnsBreakdownQuery.data, t])

  const outboundBarData: BarDatum[] = useMemo(() => {
    return (topOutboundsQuery.data ?? []).slice(0, 6).map((row) => ({
      key: row.label,
      label: row.label,
      upload: row.upload,
      download: row.download,
      total: row.total,
    }))
  }, [topOutboundsQuery.data])

  const statusHeadline = (() => {
    if (!status?.configured) return t("status.mihomoUnconfigured")
    if (!status.connected) return t("status.mihomoError")
    if (backendId === 0 && status.backendCount > 1) {
      return t("status.multiOnline", {
        online: status.connectedCount,
        total: status.backendCount,
      })
    }
    return t("status.mihomoOnline", { version: status.mihomoVersion || "" })
  })()

  return (
    <div className="space-y-5">
      {/* Live Kernel Status Strip */}
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-4 py-3.5">
          <div className="flex flex-wrap items-center gap-2.5 text-xs">
            <div className="flex items-center gap-2">
              <span
                className={
                  status?.connected
                    ? "size-2.5 rounded-full bg-success"
                    : status?.configured
                      ? "size-2.5 rounded-full bg-destructive"
                      : "size-2.5 rounded-full bg-warning"
                }
              />
              <span className="font-medium">{statusHeadline}</span>
            </div>

            {status?.connected && (
              <>
                <Badge variant="success" className="font-mono text-[11px]">
                  {t("status.transport", { value: status.transport.toUpperCase() })}
                </Badge>
                {status.mihomoMode && (
                  <Badge variant="outline" className="font-mono text-[11px]">
                    {t("status.mode", { value: status.mihomoMode })}
                  </Badge>
                )}
                {status.dnsEnhancedMode && (
                  <Badge
                    variant={status.dnsEnhancedMode === "fake-ip" ? "info" : "outline"}
                    className="font-mono text-[11px]"
                  >
                    {t("status.dns", {
                      value: status.fakeIPRange
                        ? `${status.dnsEnhancedMode} (${status.fakeIPRange})`
                        : status.dnsEnhancedMode,
                    })}
                  </Badge>
                )}
                {status.tunEnabled && (
                  <Badge variant="secondary" className="font-mono text-[11px]">
                    {t("status.tunEnabled")}
                  </Badge>
                )}
              </>
            )}

            {status?.lastError && !status.connected && (
              <span className="max-w-md truncate text-destructive" title={status.lastError}>
                {status.lastError}
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-4 text-xs tabular-nums">
            <div className="flex items-center gap-1.5">
              <ArrowUp className="size-3.5 text-chart-1" />
              <span className="text-muted-foreground">{t("status.liveUpload")}</span>
              <span className="font-mono font-semibold">
                {formatSpeed(status?.uploadSpeed ?? 0)}
              </span>
            </div>
            <div className="flex items-center gap-1.5">
              <ArrowDown className="size-3.5 text-chart-2" />
              <span className="text-muted-foreground">{t("status.liveDownload")}</span>
              <span className="font-mono font-semibold">
                {formatSpeed(status?.downloadSpeed ?? 0)}
              </span>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1.5 text-xs"
              onClick={() => navigate("/connections")}
            >
              <Radio className="size-3.5 text-success" />
              <span>
                {t("status.activeConnectionsCount", { count: status?.activeConnections ?? 0 })}
              </span>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 4 KPI Cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard
          title={t("overview.rangeTotal")}
          icon={<HardDrive className="size-4 text-muted-foreground" />}
          loading={overviewQuery.isLoading}
          value={formatBytes(overview?.totalBytes ?? 0)}
          sub={
            <span className="flex items-center gap-3 tabular-nums">
              <span className="inline-flex items-center gap-0.5">
                <ArrowUp className="size-3 text-chart-1" />
                {formatBytes(overview?.totalUpload ?? 0)}
              </span>
              <span className="inline-flex items-center gap-0.5">
                <ArrowDown className="size-3 text-chart-2" />
                {formatBytes(overview?.totalDownload ?? 0)}
              </span>
            </span>
          }
        />

        <KpiCard
          title={t("overview.fakeIPTraffic")}
          icon={<Shield className="size-4 text-info" />}
          loading={overviewQuery.isLoading}
          value={formatBytes(overview?.fakeIPBytes ?? 0)}
          sub={
            <span className="flex flex-wrap items-center justify-between gap-1.5 tabular-nums">
              <span>
                {t("overview.fakeIPRatio", {
                  ratio: formatPercent(overview?.fakeIPRatio ?? 0, i18n.language),
                })}
              </span>
              {(overview?.unresolvedFakeIPBytes ?? 0) > 0 && (
                <Badge variant="warning" className="h-4 px-1.5 text-[10px]">
                  <AlertTriangle className="size-2.5" />
                  {t("overview.unresolvedBadge", {
                    bytes: formatBytes(overview!.unresolvedFakeIPBytes),
                  })}
                </Badge>
              )}
            </span>
          }
        />

        <KpiCard
          title={t("overview.activeEntities")}
          icon={<Cpu className="size-4 text-muted-foreground" />}
          loading={overviewQuery.isLoading}
          value={`${formatNumber(overview?.processCount ?? 0, i18n.language)} / ${formatNumber(overview?.hostCount ?? 0, i18n.language)}`}
          sub={
            <span className="tabular-nums">
              {t("overview.entitiesBreakdown", {
                outbounds: overview?.outboundCount ?? 0,
                rules: overview?.ruleCount ?? 0,
                devices: overview?.sourceIPCount ?? 0,
              })}
            </span>
          }
        />

        <KpiCard
          title={t("overview.activeFakeIP")}
          icon={<Activity className="size-4 text-success" />}
          loading={statusQuery.isLoading}
          value={`${status?.activeFakeIPConnections ?? 0} / ${status?.activeConnections ?? 0}`}
          sub={
            <span className="tabular-nums">
              {(status?.unresolvedFakeIPConnections ?? 0) > 0
                ? t("overview.unresolvedActive", { count: status!.unresolvedFakeIPConnections })
                : status?.memory
                  ? t("overview.mihomoMemory", { bytes: formatBytes(status.memory) })
                  : t("overview.allResolved")}
            </span>
          }
        />
      </div>

      {/* Main Trend Chart */}
      <TrafficTrendChart
        points={trendQuery.data ?? []}
        isLoading={trendQuery.isLoading}
        isError={trendQuery.isError}
        onRetry={() => trendQuery.refetch()}
      />

      {/* Rich Visual Charts Row */}
      <div className="grid gap-4 lg:grid-cols-2">
        <TrafficDonutChart
          title={t("overview.dnsDistribution")}
          description={t("overview.dnsDistributionDesc")}
          slices={dnsSlices}
          isLoading={dnsBreakdownQuery.isLoading}
          onSelect={(key) => jumpToExplorer("dnsMode", key)}
        />
        <TrafficBarChart
          title={t("overview.topOutboundsChart")}
          description={t("overview.topOutboundsChartDesc")}
          data={outboundBarData}
          isLoading={topOutboundsQuery.isLoading}
          onSelect={(key) => jumpToExplorer("outbound", key)}
        />
      </div>

      {/* Top Processes & Top Hosts Quick Cards */}
      <div className="grid gap-4 lg:grid-cols-2">
        <TopRankCard
          title={t("overview.topProcesses")}
          icon={<Cpu className="size-4 text-muted-foreground" />}
          rows={topProcessesQuery.data}
          isLoading={topProcessesQuery.isLoading}
          onViewAll={() => jumpToExplorer("process")}
          onSelect={(label) => jumpToExplorer("process", label)}
        />
        <TopRankCard
          title={t("overview.topHosts")}
          icon={<Globe className="size-4 text-muted-foreground" />}
          rows={topHostsQuery.data}
          isLoading={topHostsQuery.isLoading}
          onViewAll={() => jumpToExplorer("host")}
          onSelect={(label) => jumpToExplorer("host", label)}
        />
      </div>
    </div>
  )
}

function KpiCard({
  title,
  icon,
  value,
  sub,
  loading,
}: {
  title: string
  icon: React.ReactNode
  value: string
  sub: React.ReactNode
  loading: boolean
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-xs font-medium text-muted-foreground">{title}</CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="space-y-2">
            <Skeleton className="h-7 w-28" />
            <Skeleton className="h-4 w-40" />
          </div>
        ) : (
          <>
            <div className="font-mono text-2xl font-semibold tracking-tight tabular-nums">
              {value}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">{sub}</div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function TopRankCard({
  title,
  icon,
  rows,
  isLoading,
  onViewAll,
  onSelect,
}: {
  title: string
  icon: React.ReactNode
  rows?: AggregatedRow[]
  isLoading: boolean
  onViewAll: () => void
  onSelect: (label: string) => void
}) {
  const { t } = useTranslation()
  const top5 = (rows ?? []).slice(0, 5)
  const maxTotal = Math.max(...top5.map((r) => r.total), 1)

  return (
    <Card className="flex flex-col">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-3">
        <div className="flex items-center gap-2">
          {icon}
          <CardTitle className="text-sm font-medium">{title}</CardTitle>
        </div>
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={onViewAll}>
          {t("common.viewAll")}
          <ArrowRight className="size-3" />
        </Button>
      </CardHeader>
      <CardContent className="flex-1">
        {isLoading ? (
          <div className="space-y-2.5">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : top5.length === 0 ? (
          <div className="flex h-40 items-center justify-center text-xs text-muted-foreground">
            {t("overview.noTraffic")}
          </div>
        ) : (
          <div className="space-y-2">
            {top5.map((row, idx) => {
              const dlPct = Math.round((row.download / maxTotal) * 100)
              const ulPct = Math.round((row.upload / maxTotal) * 100)
              return (
                <button
                  key={row.label}
                  type="button"
                  onClick={() => onSelect(row.label)}
                  className="group w-full rounded-md p-2 text-left transition-colors hover:bg-accent"
                >
                  <div className="flex items-center justify-between gap-2 text-xs">
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="w-4 shrink-0 font-mono text-[11px] text-muted-foreground tabular-nums">
                        {idx + 1}
                      </span>
                      <span className="truncate font-mono font-medium" title={row.label}>
                        {row.label}
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-2.5 font-mono tabular-nums">
                      <span className="hidden text-[11px] text-muted-foreground sm:inline">
                        ↑ {formatBytes(row.upload)} · ↓ {formatBytes(row.download)}
                      </span>
                      <span className="font-semibold">{formatBytes(row.total)}</span>
                    </div>
                  </div>
                  <div className="mt-1.5 ml-6 flex h-1.5 overflow-hidden rounded-full bg-muted">
                    <div className="h-full bg-chart-2" style={{ width: `${dlPct}%` }} />
                    <div className="h-full bg-chart-1" style={{ width: `${ulPct}%` }} />
                  </div>
                </button>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
