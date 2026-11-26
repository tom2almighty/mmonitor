import { useEffect, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { ArrowDown, ArrowUp, BarChart2, ChevronRight, Network, Search } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { DnsModeBadge } from "@/components/common/DnsModeBadge"
import { TrafficBarChart, type BarDatum } from "@/components/common/TrafficBarChart"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import type { AggregatedRow, Dimension, DisplayMode, DNSModeFilter } from "@/lib/types"

interface DrilldownDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  backendId: number
  dimension: Dimension
  primaryRow: AggregatedRow | null
  domainGroupingEnabled: boolean
  start: number
  end: number
  mode: DisplayMode
  dnsMode: DNSModeFilter
}

export function DrilldownDialog({
  open,
  onOpenChange,
  backendId,
  dimension,
  primaryRow,
  domainGroupingEnabled,
  start,
  end,
  mode,
  dnsMode,
}: DrilldownDialogProps) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<"distribution" | "chains">("distribution")
  const [search, setSearch] = useState("")
  const [selectedSecondary, setSelectedSecondary] = useState<string | null>(null)

  const primaryLabel = primaryRow?.label ?? null
  const isSummary = mode === "summary"

  useEffect(() => {
    setActiveTab("distribution")
    setSearch("")
    setSelectedSecondary(null)
  }, [primaryLabel, dimension, mode])

  const subQuery = useQuery({
    queryKey: ["substats", backendId, dimension, primaryLabel, start, end, isSummary, dnsMode],
    queryFn: () =>
      api.getSubstats({
        backendId,
        dimension,
        label: primaryLabel!,
        start,
        end,
        summary: isSummary,
        dnsMode,
      }),
    enabled: open && Boolean(primaryLabel),
  })

  const subRows = subQuery.data ?? []

  useEffect(() => {
    if (!selectedSecondary && subRows.length > 0) {
      setSelectedSecondary(subRows[0].label)
    }
  }, [subRows, selectedSecondary])

  const filteredSubRows = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return subRows
    return subRows.filter((r) => r.label.toLowerCase().includes(q))
  }, [subRows, search])

  const barData: BarDatum[] = useMemo(() => {
    return subRows.slice(0, 10).map((r) => ({
      key: r.label,
      label: r.label,
      upload: r.upload,
      download: r.download,
      total: r.total,
    }))
  }, [subRows])

  const detailQuery = useQuery({
    queryKey: [
      "details",
      backendId,
      dimension,
      primaryLabel,
      selectedSecondary,
      start,
      end,
      dnsMode,
    ],
    queryFn: () =>
      api.getConnectionDetails({
        backendId,
        dimension,
        primary: primaryLabel!,
        secondary: selectedSecondary!,
        start,
        end,
        dnsMode,
      }),
    enabled:
      open &&
      !isSummary &&
      activeTab === "chains" &&
      Boolean(primaryLabel) &&
      Boolean(selectedSecondary),
  })

  const secondaryTitle =
    dimension === "host"
      ? domainGroupingEnabled
        ? t("explorer.drilldown.subdomains")
        : t("explorer.drilldown.processes")
      : t("explorer.drilldown.targetHosts")

  const displayPrimary =
    dimension === "dnsMode" && primaryLabel
      ? t(`dnsMode.${primaryLabel as "fake-ip" | "fake-ip-unresolved" | "redir-host" | "normal"}`, {
          defaultValue: primaryLabel,
        })
      : primaryLabel

  const maxSubTotal = Math.max(...filteredSubRows.map((r) => r.total), 1)

  const inspectChains = (secondary: string) => {
    setSelectedSecondary(secondary)
    if (!isSummary) {
      setActiveTab("chains")
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] w-[94vw] max-w-4xl flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
        {/* Fixed Header */}
        <DialogHeader className="shrink-0 border-b px-5 py-4">
          <div className="flex flex-wrap items-center gap-2 pr-8">
            <Badge variant="outline" className="text-[11px]">
              {t(`explorer.dimensions.${dimension}`)}
            </Badge>
            <DialogTitle className="truncate font-mono text-base" title={displayPrimary ?? ""}>
              {displayPrimary || "--"}
            </DialogTitle>
          </div>
          <DialogDescription className="flex flex-wrap items-center gap-3 pt-1 text-xs tabular-nums">
            {primaryRow && (
              <>
                <span className="font-mono font-semibold text-foreground">
                  {t("common.total")}: {formatBytes(primaryRow.total)}
                </span>
                <span className="inline-flex items-center gap-1 font-mono">
                  <ArrowUp className="size-3 text-chart-1" />
                  {formatBytes(primaryRow.upload)}
                </span>
                <span className="inline-flex items-center gap-1 font-mono">
                  <ArrowDown className="size-3 text-chart-2" />
                  {formatBytes(primaryRow.download)}
                </span>
              </>
            )}
            {isSummary && <span>· {t("explorer.drilldown.summaryModeHint")}</span>}
          </DialogDescription>
        </DialogHeader>

        {/* Tabbed Body */}
        <Tabs
          value={activeTab}
          onValueChange={(v) => setActiveTab(v as "distribution" | "chains")}
          className="flex min-h-0 flex-1 flex-col gap-0"
        >
          <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-5 py-2.5">
            <TabsList>
              <TabsTrigger value="distribution" className="gap-1.5 text-xs">
                <BarChart2 className="size-3.5" />
                {t("explorer.drilldown.tabDistribution")}
                <Badge variant="secondary" className="ml-1 h-4 px-1.5 font-mono text-[10px]">
                  {subRows.length}
                </Badge>
              </TabsTrigger>
              {!isSummary && (
                <TabsTrigger value="chains" className="gap-1.5 text-xs">
                  <Network className="size-3.5" />
                  {t("explorer.drilldown.tabChains")}
                </TabsTrigger>
              )}
            </TabsList>

            {activeTab === "distribution" ? (
              <div className="relative w-full sm:w-56">
                <Search className="pointer-events-none absolute left-2.5 top-2 size-3.5 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={t("explorer.drilldown.filterSub")}
                  className="h-7 bg-background pl-8 text-xs"
                />
              </div>
            ) : (
              <Select
                value={selectedSecondary ?? ""}
                onValueChange={(v) => v && setSelectedSecondary(v)}
              >
                <SelectTrigger className="h-7 w-full max-w-xs font-mono text-xs">
                  <SelectValue>
                    {selectedSecondary || t("explorer.drilldown.selectSubHint")}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {subRows.slice(0, 80).map((r) => (
                    <SelectItem key={r.label} value={r.label} className="font-mono text-xs">
                      {r.label} ({formatBytes(r.total)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          {/* Tab 1: Distribution BarChart + Clean Vertical List */}
          <TabsContent value="distribution" className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
            <div className="space-y-4">
              {barData.length > 0 && (
                <TrafficBarChart
                  title={t("explorer.drilldown.topSubChartTitle", { title: secondaryTitle })}
                  data={barData}
                  isLoading={subQuery.isLoading}
                  onSelect={!isSummary ? inspectChains : undefined}
                  height={210}
                />
              )}

              {subQuery.isLoading ? (
                <div className="space-y-2">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <Skeleton key={i} className="h-12 w-full rounded-md" />
                  ))}
                </div>
              ) : subQuery.isError ? (
                <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-8 text-xs text-muted-foreground">
                  <span>{t("explorer.drilldown.loadSubError")}</span>
                  <Button variant="outline" size="sm" onClick={() => subQuery.refetch()}>
                    {t("common.retry")}
                  </Button>
                </div>
              ) : filteredSubRows.length === 0 ? (
                <div className="rounded-lg border border-dashed py-10 text-center text-xs text-muted-foreground">
                  {t("explorer.drilldown.emptySub")}
                </div>
              ) : (
                <div className="space-y-1.5">
                  {filteredSubRows.slice(0, 100).map((row) => {
                    const dlPct = Math.round((row.download / maxSubTotal) * 100)
                    const ulPct = Math.round((row.upload / maxSubTotal) * 100)
                    return (
                      <div
                        key={row.label}
                        className="flex items-center justify-between gap-3 rounded-lg border bg-card px-3.5 py-2.5 text-xs transition-colors hover:bg-accent/50"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center justify-between gap-2">
                            <span className="truncate font-mono font-medium" title={row.label}>
                              {row.label}
                            </span>
                            <span className="shrink-0 font-mono font-semibold tabular-nums">
                              {formatBytes(row.total)}
                            </span>
                          </div>
                          <div className="mt-1.5 flex items-center gap-3">
                            <div className="flex h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                              <div className="h-full bg-chart-2" style={{ width: `${dlPct}%` }} />
                              <div className="h-full bg-chart-1" style={{ width: `${ulPct}%` }} />
                            </div>
                            <span className="shrink-0 font-mono text-[11px] text-muted-foreground tabular-nums">
                              ↑ {formatBytes(row.upload)} · ↓ {formatBytes(row.download)}
                            </span>
                          </div>
                        </div>
                        {!isSummary && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 shrink-0 gap-1 px-2 text-xs"
                            onClick={() => inspectChains(row.label)}
                          >
                            {t("explorer.drilldown.viewChains")}
                            <ChevronRight className="size-3.5" />
                          </Button>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </TabsContent>

          {/* Tab 2: Connection Chains Cards */}
          {!isSummary && (
            <TabsContent value="chains" className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
              {!selectedSecondary ? (
                <div className="rounded-lg border border-dashed py-12 text-center text-xs text-muted-foreground">
                  {t("explorer.drilldown.selectSubHint")}
                </div>
              ) : detailQuery.isLoading ? (
                <div className="grid gap-3 sm:grid-cols-2">
                  {Array.from({ length: 4 }).map((_, i) => (
                    <Skeleton key={i} className="h-28 w-full rounded-lg" />
                  ))}
                </div>
              ) : detailQuery.isError ? (
                <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-10 text-xs text-muted-foreground">
                  <span>{t("explorer.drilldown.loadDetailError")}</span>
                  <Button variant="outline" size="sm" onClick={() => detailQuery.refetch()}>
                    {t("common.retry")}
                  </Button>
                </div>
              ) : (detailQuery.data?.length ?? 0) === 0 ? (
                <div className="rounded-lg border border-dashed py-12 text-center text-xs text-muted-foreground">
                  {t("explorer.drilldown.emptyDetail")}
                </div>
              ) : (
                <div className="grid gap-3 sm:grid-cols-2">
                  {detailQuery.data!.slice(0, 80).map((item, idx) => (
                    <div
                      key={`${item.destinationIP}-${item.sourceIP}-${item.process}-${item.outbound}-${item.dnsMode}-${idx}`}
                      className="rounded-lg border bg-card p-3.5 text-xs"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0 space-y-1">
                          <div className="flex flex-wrap items-center gap-1.5">
                            <span className="truncate font-mono font-medium">
                              {item.destinationIP || t("explorer.drilldown.remoteResolve")}
                            </span>
                            <DnsModeBadge mode={item.dnsMode} />
                            <Badge variant="outline" className="font-mono text-[10px] uppercase">
                              {item.network}
                            </Badge>
                          </div>
                          <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted-foreground">
                            <span>
                              {t("explorer.drilldown.processLabel")}:{" "}
                              <code className="font-mono text-foreground">{item.process}</code>
                            </span>
                            <span>
                              {t("explorer.drilldown.sourceIPLabel")}:{" "}
                              <code className="font-mono text-foreground">{item.sourceIP}</code>
                            </span>
                            {item.ruleGroup && item.ruleGroup !== "unknown" && (
                              <span>
                                {t("explorer.drilldown.ruleLabel")}:{" "}
                                <code className="font-mono text-foreground">{item.ruleGroup}</code>
                              </span>
                            )}
                          </div>
                        </div>
                        <div className="shrink-0 text-right tabular-nums">
                          <div className="font-mono font-semibold">{formatBytes(item.total)}</div>
                          <div className="text-[11px] text-muted-foreground">
                            ↑ {formatBytes(item.upload)} · ↓ {formatBytes(item.download)}
                          </div>
                        </div>
                      </div>
                      <div className="mt-2.5 flex flex-wrap items-center gap-1">
                        {(item.chains?.length ? item.chains : ["DIRECT"]).map((hop, hopIdx) => (
                          <Badge
                            key={`${hop}-${hopIdx}`}
                            variant={hopIdx === 0 ? "default" : "secondary"}
                            className="font-mono text-[10px]"
                          >
                            {hop}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </TabsContent>
          )}
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
