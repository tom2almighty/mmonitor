import { useTranslation } from "react-i18next"
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip as RechartsTooltip } from "recharts"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { formatBytes, formatPercent } from "@/lib/format"

export interface DonutSlice {
  key: string
  name: string
  value: number
  color: string
  clickable?: boolean
}

interface TrafficDonutChartProps {
  title: string
  description?: string
  slices: DonutSlice[]
  isLoading: boolean
  onSelect?: (key: string) => void
  height?: number
}

export function TrafficDonutChart({
  title,
  description,
  slices,
  isLoading,
  onSelect,
  height = 220,
}: TrafficDonutChartProps) {
  const { t, i18n } = useTranslation()
  const nonZero = slices.filter((s) => s.value > 0)
  const total = nonZero.reduce((sum, s) => sum + s.value, 0)

  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        {description && <CardDescription className="text-xs">{description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex flex-1 items-center">
        {isLoading ? (
          <Skeleton className="w-full rounded-md" style={{ height }} />
        ) : nonZero.length === 0 ? (
          <div
            className="flex w-full items-center justify-center text-xs text-muted-foreground"
            style={{ height }}
          >
            {t("common.noData")}
          </div>
        ) : (
          <div className="grid w-full items-center gap-4 sm:grid-cols-[180px_1fr]">
            <div className="relative mx-auto w-45" style={{ height }}>
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={nonZero}
                    dataKey="value"
                    nameKey="name"
                    cx="50%"
                    cy="50%"
                    innerRadius={54}
                    outerRadius={80}
                    paddingAngle={2}
                    strokeWidth={0}
                    isAnimationActive={false}
                    onClick={(entry: DonutSlice) => {
                      if (onSelect && entry.clickable !== false) {
                        onSelect(entry.key)
                      }
                    }}
                  >
                    {nonZero.map((entry) => (
                      <Cell
                        key={entry.key}
                        fill={entry.color}
                        className={
                          onSelect && entry.clickable !== false ? "cursor-pointer" : undefined
                        }
                      />
                    ))}
                  </Pie>
                  <RechartsTooltip
                    content={({ active, payload }) => {
                      if (!active || !payload?.length) return null
                      const item = payload[0].payload as DonutSlice
                      return (
                        <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
                          <div className="font-medium">{item.name}</div>
                          <div className="mt-1 flex items-center gap-2 font-mono tabular-nums">
                            <span className="font-semibold">{formatBytes(item.value)}</span>
                            <span className="text-muted-foreground">
                              ({formatPercent(total > 0 ? item.value / total : 0, i18n.language)})
                            </span>
                          </div>
                        </div>
                      )
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
                <span className="text-[10px] text-muted-foreground">{t("common.total")}</span>
                <span className="font-mono text-xs font-semibold tabular-nums">
                  {formatBytes(total)}
                </span>
              </div>
            </div>

            <div className="min-w-0 space-y-1.5">
              {nonZero.map((slice) => {
                const ratio = total > 0 ? slice.value / total : 0
                const clickable = Boolean(onSelect && slice.clickable !== false)
                return (
                  <button
                    key={slice.key}
                    type="button"
                    disabled={!clickable}
                    onClick={() => clickable && onSelect?.(slice.key)}
                    className={
                      clickable
                        ? "flex w-full items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-xs transition-colors hover:bg-accent"
                        : "flex w-full cursor-default items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-xs"
                    }
                  >
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        className="size-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: slice.color }}
                      />
                      <span className="truncate font-mono" title={slice.name}>
                        {slice.name}
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-2 font-mono text-[11px] tabular-nums">
                      <span className="font-medium">{formatBytes(slice.value)}</span>
                      <span className="w-11 text-right text-muted-foreground">
                        {formatPercent(ratio, i18n.language)}
                      </span>
                    </div>
                  </button>
                )
              })}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
