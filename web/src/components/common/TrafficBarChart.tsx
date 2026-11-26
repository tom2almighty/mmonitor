import { useTranslation } from "react-i18next"
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from "recharts"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { formatBytes } from "@/lib/format"

export interface BarDatum {
  key: string
  label: string
  upload: number
  download: number
  total: number
}

interface TrafficBarChartProps {
  title: string
  description?: string
  data: BarDatum[]
  isLoading: boolean
  onSelect?: (key: string) => void
  height?: number
}

function truncateLabel(value: string, max = 18): string {
  if (value.length <= max) return value
  return `${value.slice(0, max - 1)}…`
}

export function TrafficBarChart({
  title,
  description,
  data,
  isLoading,
  onSelect,
  height = 220,
}: TrafficBarChartProps) {
  const { t } = useTranslation()

  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        {description && <CardDescription className="text-xs">{description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex-1">
        {isLoading ? (
          <Skeleton className="w-full rounded-md" style={{ height }} />
        ) : data.length === 0 ? (
          <div
            className="flex w-full items-center justify-center text-xs text-muted-foreground"
            style={{ height }}
          >
            {t("common.noData")}
          </div>
        ) : (
          <div className="w-full" style={{ height }}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart
                data={data}
                layout="vertical"
                margin={{ top: 4, right: 16, left: 4, bottom: 0 }}
                barCategoryGap="24%"
              >
                <CartesianGrid strokeDasharray="3 3" horizontal={false} stroke="var(--border)" />
                <XAxis
                  type="number"
                  tickLine={false}
                  axisLine={false}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 11 }}
                  tickFormatter={(v: number) => formatBytes(v)}
                />
                <YAxis
                  type="category"
                  dataKey="label"
                  width={126}
                  tickLine={false}
                  axisLine={false}
                  tick={{ fill: "var(--foreground)", fontSize: 11 }}
                  tickFormatter={(v: string) => truncateLabel(v)}
                />
                <RechartsTooltip
                  cursor={{ fill: "var(--accent)", opacity: 0.45 }}
                  content={({ active, payload }) => {
                    if (!active || !payload?.length) return null
                    const row = payload[0].payload as BarDatum
                    return (
                      <div className="rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md">
                        <div className="mb-1.5 max-w-xs break-all font-mono font-medium">
                          {row.label}
                        </div>
                        <div className="space-y-1 tabular-nums">
                          <div className="flex items-center justify-between gap-4">
                            <span className="text-muted-foreground">{t("common.total")}</span>
                            <span className="font-mono font-semibold">
                              {formatBytes(row.total)}
                            </span>
                          </div>
                          <div className="flex items-center justify-between gap-4">
                            <span className="flex items-center gap-1.5">
                              <span
                                className="size-2 rounded-full"
                                style={{ backgroundColor: "var(--chart-2)" }}
                              />
                              {t("common.download")}
                            </span>
                            <span className="font-mono">{formatBytes(row.download)}</span>
                          </div>
                          <div className="flex items-center justify-between gap-4">
                            <span className="flex items-center gap-1.5">
                              <span
                                className="size-2 rounded-full"
                                style={{ backgroundColor: "var(--chart-1)" }}
                              />
                              {t("common.upload")}
                            </span>
                            <span className="font-mono">{formatBytes(row.upload)}</span>
                          </div>
                        </div>
                      </div>
                    )
                  }}
                />
                <Bar
                  dataKey="download"
                  stackId="traffic"
                  fill="var(--chart-2)"
                  radius={[0, 0, 0, 0]}
                  isAnimationActive={false}
                  className={onSelect ? "cursor-pointer" : undefined}
                  onClick={(entry) => {
                    const key = (entry as unknown as BarDatum)?.key
                    if (key && onSelect) onSelect(key)
                  }}
                />
                <Bar
                  dataKey="upload"
                  stackId="traffic"
                  fill="var(--chart-1)"
                  radius={[0, 4, 4, 0]}
                  isAnimationActive={false}
                  className={onSelect ? "cursor-pointer" : undefined}
                  onClick={(entry) => {
                    const key = (entry as unknown as BarDatum)?.key
                    if (key && onSelect) onSelect(key)
                  }}
                />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
