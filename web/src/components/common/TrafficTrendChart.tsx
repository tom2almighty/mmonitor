import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import { BarChart3, TrendingUp } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { formatBytes, formatDateTime, formatShortTime } from "@/lib/format"
import type { TrendPoint } from "@/lib/types"

interface TrafficTrendChartProps {
  points: TrendPoint[]
  isLoading: boolean
  isError: boolean
  onRetry?: () => void
  title?: string
}

type SeriesMode = "split" | "total"
type ChartKind = "line" | "bar"

interface ChartRow {
  timestamp: number
  upload: number
  download: number
  total: number
}

export function TrafficTrendChart({
  points,
  isLoading,
  isError,
  onRetry,
  title,
}: TrafficTrendChartProps) {
  const { t, i18n } = useTranslation()
  const [seriesMode, setSeriesMode] = useState<SeriesMode>("split")
  const [chartKind, setChartKind] = useState<ChartKind>("line")

  const chartData: ChartRow[] = useMemo(() => {
    return points.map((p) => ({
      timestamp: p.timestamp,
      upload: p.upload,
      download: p.download,
      total: p.upload + p.download,
    }))
  }, [points])

  const spanMs = useMemo(() => {
    if (points.length < 2) return 0
    return Math.max(0, points[points.length - 1].timestamp - points[0].timestamp)
  }, [points])

  const renderTooltip = ({
    active,
    payload,
  }: {
    active?: boolean
    payload?: ReadonlyArray<{ payload?: ChartRow }>
  }) => {
    if (!active || !payload?.length || !payload[0].payload) return null
    const row = payload[0].payload
    return (
      <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
        <div className="mb-1.5 font-medium tabular-nums text-muted-foreground">
          {formatDateTime(row.timestamp, i18n.language)}
        </div>
        <div className="space-y-1 tabular-nums">
          <div className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: "var(--chart-3)" }} />
              {t("common.total")}
            </span>
            <span className="font-mono font-semibold">{formatBytes(row.total)}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: "var(--chart-2)" }} />
              {t("common.download")}
            </span>
            <span className="font-mono">{formatBytes(row.download)}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ backgroundColor: "var(--chart-1)" }} />
              {t("common.upload")}
            </span>
            <span className="font-mono">{formatBytes(row.upload)}</span>
          </div>
        </div>
      </div>
    )
  }

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 space-y-0 pb-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-0.5">
          <CardTitle className="text-base font-medium">
            {title ?? t("overview.trendTitle")}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {seriesMode === "split" ? t("overview.trendSplitDesc") : t("overview.trendTotalDesc")}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Line / Bar switch */}
          <div className="flex items-center gap-0.5 rounded-lg border bg-muted/40 p-0.5">
            <Button
              type="button"
              variant={chartKind === "line" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 gap-1 px-2.5 text-xs"
              onClick={() => setChartKind("line")}
            >
              <TrendingUp className="size-3.5" />
              {t("overview.chartLine")}
            </Button>
            <Button
              type="button"
              variant={chartKind === "bar" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 gap-1 px-2.5 text-xs"
              onClick={() => setChartKind("bar")}
            >
              <BarChart3 className="size-3.5" />
              {t("overview.chartBar")}
            </Button>
          </div>

          {/* Split / Total switch */}
          <div className="flex items-center gap-0.5 rounded-lg border bg-muted/40 p-0.5">
            <Button
              type="button"
              variant={seriesMode === "split" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2.5 text-xs"
              onClick={() => setSeriesMode("split")}
            >
              {t("overview.trendSplit")}
            </Button>
            <Button
              type="button"
              variant={seriesMode === "total" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2.5 text-xs"
              onClick={() => setSeriesMode("total")}
            >
              {t("overview.trendTotal")}
            </Button>
          </div>
        </div>
      </CardHeader>

      <CardContent>
        {isLoading ? (
          <Skeleton className="h-65 w-full rounded-md" />
        ) : isError ? (
          <div className="flex h-65 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <span>{t("overview.trendError")}</span>
            {onRetry && (
              <Button variant="outline" size="sm" onClick={onRetry}>
                {t("common.retry")}
              </Button>
            )}
          </div>
        ) : chartData.length === 0 ? (
          <div className="flex h-65 items-center justify-center text-sm text-muted-foreground">
            {t("overview.trendEmpty")}
          </div>
        ) : (
          <div className="h-65 w-full">
            <ResponsiveContainer width="100%" height="100%">
              {chartKind === "line" ? (
                <AreaChart data={chartData} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="trendFillDownload" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="var(--chart-2)" stopOpacity={0.32} />
                      <stop offset="95%" stopColor="var(--chart-2)" stopOpacity={0.02} />
                    </linearGradient>
                    <linearGradient id="trendFillUpload" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.32} />
                      <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0.02} />
                    </linearGradient>
                    <linearGradient id="trendFillTotal" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="var(--chart-3)" stopOpacity={0.35} />
                      <stop offset="95%" stopColor="var(--chart-3)" stopOpacity={0.02} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" />
                  <XAxis
                    dataKey="timestamp"
                    tickLine={false}
                    axisLine={false}
                    minTickGap={36}
                    tickMargin={8}
                    tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                    tickFormatter={(value: number) => formatShortTime(value, spanMs, i18n.language)}
                  />
                  <YAxis
                    tickLine={false}
                    axisLine={false}
                    width={68}
                    tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                    tickFormatter={(value: number) => formatBytes(value)}
                  />
                  <RechartsTooltip content={renderTooltip} />
                  {/* Direct children (no React.Fragment!) so Recharts 2.x always registers both series */}
                  {seriesMode === "split" && (
                    <Area
                      type="monotone"
                      dataKey="download"
                      stroke="var(--chart-2)"
                      strokeWidth={2}
                      fill="url(#trendFillDownload)"
                      isAnimationActive={false}
                    />
                  )}
                  {seriesMode === "split" && (
                    <Area
                      type="monotone"
                      dataKey="upload"
                      stroke="var(--chart-1)"
                      strokeWidth={1.75}
                      fill="url(#trendFillUpload)"
                      isAnimationActive={false}
                    />
                  )}
                  {seriesMode === "total" && (
                    <Area
                      type="monotone"
                      dataKey="total"
                      stroke="var(--chart-3)"
                      strokeWidth={2}
                      fill="url(#trendFillTotal)"
                      isAnimationActive={false}
                    />
                  )}
                </AreaChart>
              ) : (
                <BarChart data={chartData} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" />
                  <XAxis
                    dataKey="timestamp"
                    tickLine={false}
                    axisLine={false}
                    minTickGap={36}
                    tickMargin={8}
                    tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                    tickFormatter={(value: number) => formatShortTime(value, spanMs, i18n.language)}
                  />
                  <YAxis
                    tickLine={false}
                    axisLine={false}
                    width={68}
                    tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                    tickFormatter={(value: number) => formatBytes(value)}
                  />
                  <RechartsTooltip
                    cursor={{ fill: "var(--accent)", opacity: 0.4 }}
                    content={renderTooltip}
                  />
                  {seriesMode === "split" && (
                    <Bar
                      dataKey="download"
                      stackId="traffic"
                      fill="var(--chart-2)"
                      radius={[0, 0, 0, 0]}
                      isAnimationActive={false}
                    />
                  )}
                  {seriesMode === "split" && (
                    <Bar
                      dataKey="upload"
                      stackId="traffic"
                      fill="var(--chart-1)"
                      radius={[3, 3, 0, 0]}
                      isAnimationActive={false}
                    />
                  )}
                  {seriesMode === "total" && (
                    <Bar
                      dataKey="total"
                      fill="var(--chart-3)"
                      radius={[3, 3, 0, 0]}
                      isAnimationActive={false}
                    />
                  )}
                </BarChart>
              )}
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
