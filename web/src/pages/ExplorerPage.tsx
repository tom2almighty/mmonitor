import { useEffect, useMemo, useState } from "react"
import { useSearchParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  BarChart2,
  ChevronLeft,
  ChevronRight,
  Cpu,
  Globe,
  Laptop,
  Search,
  Shield,
  SlidersHorizontal,
  Waypoints,
} from "lucide-react"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { DnsModeBadge } from "@/components/common/DnsModeBadge"
import { TrafficBarChart, type BarDatum } from "@/components/common/TrafficBarChart"
import { TrafficDonutChart, type DonutSlice } from "@/components/common/TrafficDonutChart"
import { DrilldownDialog } from "@/components/explorer/DrilldownDialog"
import { api } from "@/lib/api"
import { formatBytes, formatNumber, formatPercent } from "@/lib/format"
import { useFilters } from "@/hooks/use-filter-context"
import type { AggregatedRow, Dimension } from "@/lib/types"

const DIM_STORAGE_KEY = "mmonitor:selected-dimension"
const PAGE_SIZE = 25

const dimensionMeta: Array<{
  id: Dimension
  icon: React.ComponentType<{ className?: string }>
}> = [
  { id: "process", icon: Cpu },
  { id: "host", icon: Globe },
  { id: "outbound", icon: Waypoints },
  { id: "rule", icon: SlidersHorizontal },
  { id: "sourceIP", icon: Laptop },
  { id: "dnsMode", icon: Shield },
]

const donutPalette = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
  "var(--info)",
  "var(--muted-foreground)",
]

type SortKey = "total" | "download" | "upload" | "count" | "label"

function isValidDimension(val: string | null): val is Dimension {
  return Boolean(val && dimensionMeta.some((d) => d.id === val))
}

export function ExplorerPage() {
  const { t, i18n } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const { backendId, start, end, mode, dnsMode } = useFilters()
  const isSummary = mode === "summary"

  const urlDim = searchParams.get("dimension")
  const urlPrimary = searchParams.get("primary")

  const [dimension, setDimension] = useState<Dimension>(() => {
    if (isValidDimension(urlDim)) return urlDim
    try {
      const saved = window.localStorage.getItem(DIM_STORAGE_KEY)
      if (isValidDimension(saved)) return saved
    } catch {
      // ignore
    }
    return "process"
  })

  const [search, setSearch] = useState("")
  const [sortKey, setSortKey] = useState<SortKey>("total")
  const [sortAsc, setSortAsc] = useState(false)
  const [page, setPage] = useState(1)
  const [showCharts, setShowCharts] = useState(true)
  const [selectedLabel, setSelectedLabel] = useState<string | null>(urlPrimary)
  const [dialogOpen, setDialogOpen] = useState(Boolean(urlPrimary))

  useEffect(() => {
    if (isValidDimension(urlDim) && urlDim !== dimension) {
      setDimension(urlDim)
    }
    if (urlPrimary) {
      setSelectedLabel(urlPrimary)
      setDialogOpen(true)
    }
  }, [urlDim, urlPrimary, dimension])

  const prefsQuery = useQuery({
    queryKey: ["preferences"],
    queryFn: api.getPreferences,
    staleTime: 60_000,
  })

  const aggQuery = useQuery({
    queryKey: ["aggregate", backendId, dimension, start, end, isSummary, dnsMode],
    queryFn: () =>
      api.getAggregate({ backendId, dimension, start, end, summary: isSummary, dnsMode }),
  })

  const dimensionLabel = t(`explorer.dimensions.${dimension}`)

  const formatRowLabel = (label: string) => {
    if (dimension !== "dnsMode") return label
    if (label === "fake-ip") return t("dnsMode.fake-ip")
    if (label === "fake-ip-unresolved") return t("dnsMode.fake-ip-unresolved")
    if (label === "redir-host") return t("dnsMode.redir-host")
    if (label === "normal") return t("dnsMode.normal")
    return label
  }

  const handleDimensionChange = (next: Dimension) => {
    setDimension(next)
    setPage(1)
    setSearch("")
    setSelectedLabel(null)
    setDialogOpen(false)
    setSearchParams({ dimension: next }, { replace: true })
    try {
      window.localStorage.setItem(DIM_STORAGE_KEY, next)
    } catch {
      // ignore
    }
  }

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortAsc((prev) => !prev)
    } else {
      setSortKey(key)
      setSortAsc(key === "label")
    }
    setPage(1)
  }

  const allRows = aggQuery.data ?? []
  const grandTotal = useMemo(() => allRows.reduce((acc, r) => acc + r.total, 0), [allRows])

  const selectedRow: AggregatedRow | null = useMemo(() => {
    if (!selectedLabel) return null
    return (
      allRows.find((r) => r.label === selectedLabel) ?? {
        label: selectedLabel,
        upload: 0,
        download: 0,
        total: 0,
        count: 0,
      }
    )
  }, [allRows, selectedLabel])

  const barData: BarDatum[] = useMemo(() => {
    return allRows.slice(0, 10).map((row) => ({
      key: row.label,
      label: formatRowLabel(row.label),
      upload: row.upload,
      download: row.download,
      total: row.total,
    }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allRows, dimension, i18n.language])

  const donutSlices: DonutSlice[] = useMemo(() => {
    const top6 = allRows.slice(0, 6)
    const restTotal = allRows.slice(6).reduce((sum, r) => sum + r.total, 0)
    const slices: DonutSlice[] = top6.map((row, idx) => ({
      key: row.label,
      name: formatRowLabel(row.label),
      value: row.total,
      color: donutPalette[idx % donutPalette.length],
      clickable: true,
    }))
    if (restTotal > 0) {
      slices.push({
        key: "__other__",
        name: t("common.other"),
        value: restTotal,
        color: "var(--muted-foreground)",
        clickable: false,
      })
    }
    return slices
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allRows, dimension, i18n.language])

  const filteredAndSorted = useMemo(() => {
    const q = search.trim().toLowerCase()
    const list = allRows.filter((row) => {
      if (!q) return true
      return (
        row.label.toLowerCase().includes(q) || formatRowLabel(row.label).toLowerCase().includes(q)
      )
    })

    return [...list].sort((a, b) => {
      const mul = sortAsc ? 1 : -1
      if (sortKey === "label") return a.label.localeCompare(b.label) * mul
      return (a[sortKey] - b[sortKey]) * mul
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allRows, search, sortKey, sortAsc, dimension, i18n.language])

  const totalPages = Math.max(1, Math.ceil(filteredAndSorted.length / PAGE_SIZE))
  const safePage = Math.min(page, totalPages)
  const pageRows = filteredAndSorted.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)

  const openDrilldown = (label: string) => {
    setSelectedLabel(label)
    setDialogOpen(true)
    setSearchParams({ dimension, primary: label }, { replace: true })
  }

  return (
    <div className="space-y-4">
      {/* Dimension Selector + Chart Toggle */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={dimension} onValueChange={(v) => handleDimensionChange(v as Dimension)}>
          <TabsList className="flex h-auto flex-wrap justify-start">
            {dimensionMeta.map((d) => {
              const Icon = d.icon
              return (
                <TabsTrigger key={d.id} value={d.id} className="gap-1.5 text-xs">
                  <Icon className="size-3.5" />
                  {t(`explorer.dimensions.${d.id}`)}
                </TabsTrigger>
              )
            })}
          </TabsList>
        </Tabs>

        <Button
          variant="outline"
          size="sm"
          className="h-8 gap-1.5 text-xs"
          onClick={() => setShowCharts((v) => !v)}
        >
          <BarChart2 className="size-3.5" />
          {showCharts ? t("explorer.toggleChartsHide") : t("explorer.toggleChartsShow")}
        </Button>
      </div>

      {/* Visual Charts Overview */}
      {showCharts && (
        <div className="grid gap-4 lg:grid-cols-2">
          <TrafficBarChart
            title={t("explorer.topBarTitle", { dimension: dimensionLabel })}
            description={t("explorer.topBarDesc")}
            data={barData}
            isLoading={aggQuery.isLoading}
            onSelect={openDrilldown}
            height={250}
          />
          <TrafficDonutChart
            title={t("explorer.sharePieTitle", { dimension: dimensionLabel })}
            description={t("explorer.sharePieDesc")}
            slices={donutSlices}
            isLoading={aggQuery.isLoading}
            onSelect={openDrilldown}
            height={250}
          />
        </div>
      )}

      {/* Primary Data Table */}
      <Card>
        <CardHeader className="flex flex-col gap-3 space-y-0 pb-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Badge variant="secondary" className="h-7 px-2.5 font-mono text-xs tabular-nums">
              {t("common.itemCount", { count: filteredAndSorted.length })}
            </Badge>
            <span>{t("explorer.clickHint")}</span>
          </div>

          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute left-2.5 top-2.5 size-3.5 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
              placeholder={t("explorer.searchPlaceholder")}
              className="h-8 pl-8 text-xs"
            />
          </div>
        </CardHeader>

        <CardContent className="p-0">
          {aggQuery.isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : aggQuery.isError ? (
            <div className="flex flex-col items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
              <span>{t("explorer.loadError")}</span>
              <Button variant="outline" size="sm" onClick={() => aggQuery.refetch()}>
                {t("common.retry")}
              </Button>
            </div>
          ) : pageRows.length === 0 ? (
            <div className="py-16 text-center text-sm text-muted-foreground">
              {t("explorer.empty")}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-12 pl-4 text-xs">#</TableHead>
                    <TableHead className="text-xs">
                      <button
                        type="button"
                        onClick={() => toggleSort("label")}
                        className="inline-flex items-center gap-1 hover:text-foreground"
                      >
                        {t("common.name")}
                        <ArrowUpDown className="size-3" />
                      </button>
                    </TableHead>
                    <TableHead className="w-40 text-xs">{t("common.share")}</TableHead>
                    <TableHead className="w-28 text-right text-xs">
                      <button
                        type="button"
                        onClick={() => toggleSort("upload")}
                        className="inline-flex items-center gap-1 hover:text-foreground"
                      >
                        {t("common.upload")}
                        <ArrowUpDown className="size-3" />
                      </button>
                    </TableHead>
                    <TableHead className="w-28 text-right text-xs">
                      <button
                        type="button"
                        onClick={() => toggleSort("download")}
                        className="inline-flex items-center gap-1 hover:text-foreground"
                      >
                        {t("common.download")}
                        <ArrowUpDown className="size-3" />
                      </button>
                    </TableHead>
                    <TableHead className="w-28 text-right text-xs">
                      <button
                        type="button"
                        onClick={() => toggleSort("total")}
                        className="inline-flex items-center gap-1 hover:text-foreground"
                      >
                        {t("common.total")}
                        <ArrowUpDown className="size-3" />
                      </button>
                    </TableHead>
                    <TableHead className="w-24 pr-4 text-right text-xs">
                      <button
                        type="button"
                        onClick={() => toggleSort("count")}
                        className="inline-flex items-center gap-1 hover:text-foreground"
                      >
                        {t("common.samples")}
                        <ArrowUpDown className="size-3" />
                      </button>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pageRows.map((row, idx) => {
                    const rank = (safePage - 1) * PAGE_SIZE + idx + 1
                    const ratio = grandTotal > 0 ? row.total / grandTotal : 0
                    const dlRatio = grandTotal > 0 ? (row.download / grandTotal) * 100 : 0
                    const ulRatio = grandTotal > 0 ? (row.upload / grandTotal) * 100 : 0

                    return (
                      <TableRow
                        key={row.label}
                        className="cursor-pointer transition-colors hover:bg-accent/50"
                        onClick={() => openDrilldown(row.label)}
                      >
                        <TableCell className="pl-4 font-mono text-xs text-muted-foreground tabular-nums">
                          {rank}
                        </TableCell>
                        <TableCell className="max-w-xs">
                          <div className="flex items-center gap-2">
                            {dimension === "dnsMode" ? (
                              <DnsModeBadge mode={row.label} />
                            ) : (
                              <span
                                className="truncate font-mono text-xs font-medium"
                                title={row.label}
                              >
                                {row.label}
                              </span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <div className="flex h-1.5 w-20 overflow-hidden rounded-full bg-muted">
                              <div className="h-full bg-chart-2" style={{ width: `${dlRatio}%` }} />
                              <div className="h-full bg-chart-1" style={{ width: `${ulRatio}%` }} />
                            </div>
                            <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                              {formatPercent(ratio, i18n.language)}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                          <span className="inline-flex items-center gap-0.5">
                            <ArrowUp className="size-3 text-chart-1" />
                            {formatBytes(row.upload)}
                          </span>
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs text-muted-foreground tabular-nums">
                          <span className="inline-flex items-center gap-0.5">
                            <ArrowDown className="size-3 text-chart-2" />
                            {formatBytes(row.download)}
                          </span>
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs font-semibold tabular-nums">
                          {formatBytes(row.total)}
                        </TableCell>
                        <TableCell className="pr-4 text-right font-mono text-xs text-muted-foreground tabular-nums">
                          {formatNumber(row.count, i18n.language)}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between border-t px-4 py-3 text-xs text-muted-foreground">
              <span className="tabular-nums">
                {t("common.pageSummary", {
                  page: safePage,
                  totalPages,
                  total: filteredAndSorted.length,
                })}
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 px-2"
                  disabled={safePage <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  <ChevronLeft className="size-3.5" />
                  {t("common.prevPage")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 px-2"
                  disabled={safePage >= totalPages}
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                >
                  {t("common.nextPage")}
                  <ChevronRight className="size-3.5" />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Tabbed Inspection Dialog (zero horizontal table scroll) */}
      <DrilldownDialog
        open={dialogOpen}
        onOpenChange={(next) => {
          setDialogOpen(next)
          if (!next) {
            setSearchParams({ dimension }, { replace: true })
          }
        }}
        backendId={backendId}
        dimension={dimension}
        primaryRow={selectedRow}
        domainGroupingEnabled={prefsQuery.data?.domainGroupingEnabled ?? true}
        start={start}
        end={end}
        mode={mode}
        dnsMode={dnsMode}
      />
    </div>
  )
}
