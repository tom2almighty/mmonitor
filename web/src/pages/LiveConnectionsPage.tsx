import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { ArrowDown, ArrowUp, Pause, Play, Radio, Search, Trash2, X } from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { DnsModeBadge } from "@/components/common/DnsModeBadge"
import { api } from "@/lib/api"
import { formatBytes, formatSpeed } from "@/lib/format"
import { useFilters } from "@/hooks/use-filter-context"

export function LiveConnectionsPage() {
  const { t } = useTranslation()
  const { backendId } = useFilters()
  const queryClient = useQueryClient()
  const [paused, setPaused] = useState(false)
  const [search, setSearch] = useState("")
  const [dnsFilter, setDnsFilter] = useState("all")
  const [netFilter, setNetFilter] = useState("all")

  const liveQuery = useQuery({
    queryKey: ["live-connections", backendId],
    queryFn: () => api.getLiveConnections(backendId),
    refetchInterval: paused ? false : 2000,
  })

  const closeOneMutation = useMutation({
    mutationFn: ({ bId, id }: { bId: number; id: string }) => api.closeLiveConnection(bId, id),
    onSuccess: () => {
      toast.success(t("connections.closedOne"))
      void queryClient.invalidateQueries({ queryKey: ["live-connections"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const closeAllMutation = useMutation({
    mutationFn: () => api.closeAllLiveConnections(backendId),
    onSuccess: () => {
      toast.success(t("connections.closedAll"))
      void queryClient.invalidateQueries({ queryKey: ["live-connections"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const connections = liveQuery.data?.connections ?? []
  const status = liveQuery.data?.status
  const showBackendCol = backendId === 0 && (status?.backendCount ?? 0) > 1

  const dnsFilterLabels: Record<string, string> = {
    all: t("filters.dnsAll"),
    "fake-ip": t("filters.dnsFakeIP"),
    "fake-ip-unresolved": t("filters.dnsFakeIPUnresolved"),
    normal: t("filters.dnsNormal"),
  }

  const netFilterLabels: Record<string, string> = {
    all: t("connections.allProtocols"),
    tcp: "TCP",
    udp: "UDP",
  }

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return connections.filter((c) => {
      if (
        dnsFilter === "fake-ip" &&
        c.dnsMode !== "fake-ip" &&
        c.dnsMode !== "fake-ip-unresolved"
      ) {
        return false
      }
      if (dnsFilter === "fake-ip-unresolved" && c.dnsMode !== "fake-ip-unresolved") {
        return false
      }
      if (dnsFilter === "normal" && c.dnsMode !== "normal") {
        return false
      }
      if (netFilter !== "all" && c.network !== netFilter) {
        return false
      }
      if (!q) return true
      return (
        c.host.toLowerCase().includes(q) ||
        c.process.toLowerCase().includes(q) ||
        c.outbound.toLowerCase().includes(q) ||
        c.ruleGroup.toLowerCase().includes(q) ||
        c.destinationIP.toLowerCase().includes(q) ||
        c.sourceIP.toLowerCase().includes(q) ||
        c.backendName.toLowerCase().includes(q)
      )
    })
  }, [connections, search, dnsFilter, netFilter])

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-col gap-3 space-y-0 pb-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full sm:w-64">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 size-3.5 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("connections.searchPlaceholder")}
                className="h-8 pl-8 text-xs"
              />
            </div>

            <Select value={dnsFilter} onValueChange={(v) => v && setDnsFilter(v)}>
              <SelectTrigger className="h-8 min-w-36 text-xs">
                <SelectValue>{dnsFilterLabels[dnsFilter]}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all" className="text-xs">
                  {t("filters.dnsAll")}
                </SelectItem>
                <SelectItem value="fake-ip" className="text-xs">
                  {t("filters.dnsFakeIP")}
                </SelectItem>
                <SelectItem value="fake-ip-unresolved" className="text-xs">
                  {t("filters.dnsFakeIPUnresolved")}
                </SelectItem>
                <SelectItem value="normal" className="text-xs">
                  {t("filters.dnsNormal")}
                </SelectItem>
              </SelectContent>
            </Select>

            <Select value={netFilter} onValueChange={(v) => v && setNetFilter(v)}>
              <SelectTrigger className="h-8 min-w-28 text-xs">
                <SelectValue>{netFilterLabels[netFilter]}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all" className="text-xs">
                  {t("connections.allProtocols")}
                </SelectItem>
                <SelectItem value="tcp" className="text-xs">
                  TCP
                </SelectItem>
                <SelectItem value="udp" className="text-xs">
                  UDP
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="secondary"
              className="h-8 gap-1.5 px-2.5 font-mono text-xs tabular-nums"
            >
              <Radio className="size-3 text-success" />
              {t("connections.countSummary", {
                filtered: filtered.length,
                total: connections.length,
              })}
              {status && (
                <span className="text-muted-foreground">
                  · ↑ {formatSpeed(status.uploadSpeed)} · ↓ {formatSpeed(status.downloadSpeed)}
                </span>
              )}
            </Badge>

            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1.5 text-xs"
              onClick={() => setPaused((p) => !p)}
            >
              {paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
              {paused ? t("connections.resume") : t("connections.pause")}
            </Button>

            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button
                    variant="destructive"
                    size="sm"
                    className="h-8 gap-1.5 text-xs"
                    disabled={connections.length === 0 || closeAllMutation.isPending}
                  />
                }
              >
                <Trash2 className="size-3.5" />
                {t("connections.closeAll")}
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{t("connections.closeAllTitle")}</AlertDialogTitle>
                  <AlertDialogDescription>
                    {t("connections.closeAllDesc", { count: connections.length })}
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    onClick={() => closeAllMutation.mutate()}
                  >
                    {t("connections.confirmCloseAll")}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </CardHeader>

        <CardContent className="p-0">
          {liveQuery.isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : liveQuery.isError ? (
            <div className="flex flex-col items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
              <span>{t("connections.loadError")}</span>
              <Button variant="outline" size="sm" onClick={() => liveQuery.refetch()}>
                {t("common.retry")}
              </Button>
            </div>
          ) : filtered.length === 0 ? (
            <div className="py-16 text-center text-sm text-muted-foreground">
              {status?.connected
                ? t("connections.emptyConnected")
                : t("connections.emptyDisconnected")}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    {showBackendCol && (
                      <TableHead className="pl-4 text-xs">{t("connections.colBackend")}</TableHead>
                    )}
                    <TableHead className={showBackendCol ? "text-xs" : "pl-4 text-xs"}>
                      {t("connections.colHost")}
                    </TableHead>
                    <TableHead className="text-xs">{t("connections.colProcess")}</TableHead>
                    <TableHead className="text-xs">{t("connections.colDnsProto")}</TableHead>
                    <TableHead className="text-xs">{t("connections.colRule")}</TableHead>
                    <TableHead className="text-xs">{t("connections.colChains")}</TableHead>
                    <TableHead className="text-right text-xs">
                      {t("connections.colSpeed")}
                    </TableHead>
                    <TableHead className="text-right text-xs">
                      {t("connections.colTraffic")}
                    </TableHead>
                    <TableHead className="w-12 pr-4 text-right text-xs">
                      {t("common.actions")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.slice(0, 150).map((conn) => (
                    <TableRow key={`${conn.backendId}-${conn.id}`}>
                      {showBackendCol && (
                        <TableCell className="pl-4">
                          <Badge variant="outline" className="font-mono text-[10px]">
                            {conn.backendName}
                          </Badge>
                        </TableCell>
                      )}
                      <TableCell className={showBackendCol ? "max-w-60" : "max-w-65 pl-4"}>
                        <div className="truncate font-mono text-xs font-medium" title={conn.host}>
                          {conn.host}
                          {conn.destinationPort ? `:${conn.destinationPort}` : ""}
                        </div>
                        <div className="truncate font-mono text-[11px] text-muted-foreground">
                          {conn.destinationIP || t("explorer.drilldown.remoteResolve")} ·{" "}
                          {t("connections.sourcePrefix")} {conn.sourceIP}
                        </div>
                      </TableCell>
                      <TableCell className="max-w-37.5">
                        {conn.processPath ? (
                          <Tooltip>
                            <TooltipTrigger
                              render={<span className="block truncate font-mono text-xs" />}
                            >
                              {conn.process}
                            </TooltipTrigger>
                            <TooltipContent className="max-w-sm break-all font-mono text-xs">
                              {conn.processPath}
                            </TooltipContent>
                          </Tooltip>
                        ) : (
                          <span className="block truncate font-mono text-xs">{conn.process}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <DnsModeBadge mode={conn.dnsMode} />
                          <Badge variant="outline" className="font-mono text-[10px] uppercase">
                            {conn.network}
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className="max-w-40">
                        <span
                          className="block truncate font-mono text-xs text-muted-foreground"
                          title={conn.ruleGroup}
                        >
                          {conn.ruleGroup}
                        </span>
                      </TableCell>
                      <TableCell className="max-w-50">
                        <div className="flex flex-wrap gap-1">
                          {conn.chains.slice(0, 3).map((hop, i) => (
                            <Badge
                              key={`${hop}-${i}`}
                              variant={i === 0 ? "default" : "secondary"}
                              className="max-w-35 truncate font-mono text-[10px]"
                            >
                              {hop}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="text-right font-mono text-[11px] tabular-nums">
                        <div className="inline-flex items-center gap-1 text-chart-2">
                          <ArrowDown className="size-3" />
                          {formatSpeed(conn.downloadSpeed)}
                        </div>
                        <div className="ml-2 inline-flex items-center gap-1 text-muted-foreground">
                          <ArrowUp className="size-3 text-chart-1" />
                          {formatSpeed(conn.uploadSpeed)}
                        </div>
                      </TableCell>
                      <TableCell className="text-right font-mono text-xs tabular-nums">
                        <div className="font-semibold">
                          {formatBytes(conn.upload + conn.download)}
                        </div>
                        <div className="text-[11px] text-muted-foreground">
                          ↑ {formatBytes(conn.upload)} · ↓ {formatBytes(conn.download)}
                        </div>
                      </TableCell>
                      <TableCell className="pr-4 text-right">
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-7 text-muted-foreground hover:text-destructive"
                                aria-label={t("connections.closeOne")}
                                disabled={closeOneMutation.isPending}
                                onClick={() =>
                                  closeOneMutation.mutate({ bId: conn.backendId, id: conn.id })
                                }
                              />
                            }
                          >
                            <X className="size-3.5" />
                          </TooltipTrigger>
                          <TooltipContent>{t("connections.closeOne")}</TooltipContent>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
