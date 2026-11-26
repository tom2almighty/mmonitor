import { useEffect, useMemo, useState } from "react"
import { useSearchParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import type { TFunction } from "i18next"
import { enUS, zhCN } from "date-fns/locale"
import {
  ArrowDown,
  ArrowUp,
  CalendarIcon,
  Database,
  Eraser,
  HardDrive,
  History,
  Layers,
  Loader2,
  Pencil,
  Plus,
  Save,
  Server,
  Sliders,
  Sparkles,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Checkbox } from "@/components/ui/checkbox"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
import { BackendFormDialog } from "@/components/common/BackendFormDialog"
import { api } from "@/lib/api"
import { formatBytes, formatDateShort, formatNumber, formatSpeed } from "@/lib/format"
import { useFilters } from "@/hooks/use-filter-context"
import type { BackendWithStatus } from "@/lib/types"

const createPrefsSchema = (t: TFunction) =>
  z.object({
    domainGroupingEnabled: z.boolean(),
    filterDirectTraffic: z.boolean(),
    retentionDays: z.coerce
      .number()
      .int()
      .min(1, t("settings.validation.minRetention"))
      .max(365, t("settings.validation.maxRetention")),
  })

type PrefsFormValues = {
  domainGroupingEnabled: boolean
  filterDirectTraffic: boolean
  retentionDays: number
}

export function SettingsPage() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const { backendId, setBackendId } = useFilters()
  const [searchParams, setSearchParams] = useSearchParams()

  const initialTab = searchParams.get("tab")
  const [activeTab, setActiveTab] = useState(
    initialTab && ["backends", "preferences", "data"].includes(initialTab)
      ? initialTab
      : "backends",
  )

  useEffect(() => {
    const tab = searchParams.get("tab")
    if (tab && ["backends", "preferences", "data"].includes(tab) && tab !== activeTab) {
      setActiveTab(tab)
    }
  }, [searchParams, activeTab])

  // Backend dialog & delete dialog states
  const [backendDialogOpen, setBackendDialogOpen] = useState(false)
  const [editingBackend, setEditingBackend] = useState<BackendWithStatus | null>(null)
  const [deletingBackend, setDeletingBackend] = useState<BackendWithStatus | null>(null)
  const [deleteBackendData, setDeleteBackendData] = useState(false)

  // Data management states
  const [pruneDate, setPruneDate] = useState<Date>(() => {
    const d = new Date()
    d.setDate(d.getDate() - 30)
    d.setHours(0, 0, 0, 0)
    return d
  })
  const [pruneCalOpen, setPruneCalOpen] = useState(false)
  const [pruneScope, setPruneScope] = useState<"detail" | "all">("detail")
  const [confirmText, setConfirmText] = useState("")
  const [clearDialogOpen, setClearDialogOpen] = useState(false)

  const prefsSchema = useMemo(() => createPrefsSchema(t), [t])
  const calLocale = i18n.language.startsWith("zh") ? zhCN : enUS

  const backendsQuery = useQuery({
    queryKey: ["backends"],
    queryFn: api.listBackends,
    refetchInterval: 3000,
  })

  const prefsQuery = useQuery({
    queryKey: ["preferences"],
    queryFn: api.getPreferences,
  })

  const storageQuery = useQuery({
    queryKey: ["storage-stats"],
    queryFn: api.getStorageStats,
  })

  const prefsForm = useForm<PrefsFormValues>({
    resolver: zodResolver(prefsSchema),
    defaultValues: {
      domainGroupingEnabled: true,
      filterDirectTraffic: false,
      retentionDays: 30,
    },
  })

  useEffect(() => {
    if (prefsQuery.data) {
      prefsForm.reset(prefsQuery.data)
    }
  }, [prefsQuery.data, prefsForm])

  const toggleBackendMutation = useMutation({
    mutationFn: (b: BackendWithStatus) =>
      api.updateBackend(b.id, {
        name: b.name,
        url: b.url,
        enabled: !b.enabled,
      }),
    onSuccess: () => {
      toast.success(t("backends.updatedToast"))
      void queryClient.invalidateQueries({ queryKey: ["backends"] })
      void queryClient.invalidateQueries({ queryKey: ["status"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const deleteBackendMutation = useMutation({
    mutationFn: ({ id, deleteData }: { id: number; deleteData: boolean }) =>
      api.deleteBackend(id, deleteData),
    onSuccess: (_, vars) => {
      toast.success(t("backends.deletedToast"))
      if (backendId === vars.id) setBackendId(0)
      setDeletingBackend(null)
      setDeleteBackendData(false)
      void queryClient.invalidateQueries({ queryKey: ["backends"] })
      void queryClient.invalidateQueries({ queryKey: ["status"] })
      void queryClient.invalidateQueries({ queryKey: ["storage-stats"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const savePrefsMutation = useMutation({
    mutationFn: (values: PrefsFormValues) => api.updatePreferences(values),
    onSuccess: () => {
      toast.success(t("settings.prefsSaved"))
      void queryClient.invalidateQueries({ queryKey: ["preferences"] })
      void queryClient.invalidateQueries({ queryKey: ["aggregate"] })
      void queryClient.invalidateQueries({ queryKey: ["overview"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const vacuumMutation = useMutation({
    mutationFn: api.vacuumStorage,
    onSuccess: (res) => {
      toast.success(
        t("settings.storage.vacuumDone", {
          before: formatBytes(res.beforeBytes),
          after: formatBytes(res.afterBytes),
        }),
      )
      void queryClient.invalidateQueries({ queryKey: ["storage-stats"] })
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const pruneMutation = useMutation({
    mutationFn: () =>
      api.pruneStorage({
        beforeTs: pruneDate.getTime(),
        scope: pruneScope,
      }),
    onSuccess: (res) => {
      toast.success(
        t("settings.storage.pruneDone", {
          detail: formatNumber(res.detailDeleted, i18n.language),
          summary: formatNumber(res.summaryDeleted, i18n.language),
        }),
      )
      void queryClient.invalidateQueries()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const clearMutation = useMutation({
    mutationFn: () => api.clearAllTraffic(),
    onSuccess: () => {
      toast.success(t("settings.clearedToast"))
      setConfirmText("")
      setClearDialogOpen(false)
      void queryClient.invalidateQueries()
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const backends = backendsQuery.data ?? []
  const storageStats = storageQuery.data
  const earliestTS = (() => {
    const a = storageStats?.oldestSummaryTs ?? 0
    const b = storageStats?.oldestDetailTs ?? 0
    if (a > 0 && b > 0) return Math.min(a, b)
    return a || b
  })()

  const pruneScopeLabels: Record<"detail" | "all", string> = {
    detail: t("settings.storage.scopeDetail"),
    all: t("settings.storage.scopeAll"),
  }

  return (
    <div className="w-full space-y-5">
      <Tabs
        value={activeTab}
        onValueChange={(v) => {
          setActiveTab(v)
          setSearchParams({ tab: v }, { replace: true })
        }}
        className="space-y-4"
      >
        <TabsList>
          <TabsTrigger value="backends" className="gap-1.5 text-xs">
            <Server className="size-3.5" />
            {t("settings.tabBackends")}
            <Badge variant="secondary" className="ml-1 h-4 px-1.5 font-mono text-[10px]">
              {backends.length}
            </Badge>
          </TabsTrigger>
          <TabsTrigger value="preferences" className="gap-1.5 text-xs">
            <Sliders className="size-3.5" />
            {t("settings.tabPreferences")}
          </TabsTrigger>
          <TabsTrigger value="data" className="gap-1.5 text-xs">
            <Database className="size-3.5" />
            {t("settings.tabData")}
          </TabsTrigger>
        </TabsList>

        {/* Tab 1: Multi-Backend Management */}
        <TabsContent value="backends" className="space-y-4">
          <Card>
            <CardHeader className="flex flex-col gap-3 space-y-0 sm:flex-row sm:items-center sm:justify-between">
              <div className="space-y-1">
                <CardTitle className="text-base">{t("settings.backendsTitle")}</CardTitle>
                <CardDescription className="text-xs">{t("settings.backendsDesc")}</CardDescription>
              </div>
              <Button
                size="sm"
                className="gap-1.5"
                onClick={() => {
                  setEditingBackend(null)
                  setBackendDialogOpen(true)
                }}
              >
                <Plus className="size-4" />
                {t("backends.addBackend")}
              </Button>
            </CardHeader>

            <CardContent>
              {backendsQuery.isLoading ? (
                <div className="grid gap-3 md:grid-cols-2">
                  {Array.from({ length: 2 }).map((_, i) => (
                    <Skeleton key={i} className="h-32 w-full rounded-lg" />
                  ))}
                </div>
              ) : backends.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-14 text-center">
                  <Server className="size-8 text-muted-foreground/60" />
                  <div className="space-y-1">
                    <div className="text-sm font-medium">{t("backends.emptyTitle")}</div>
                    <p className="max-w-md text-xs text-muted-foreground">
                      {t("backends.emptyDesc")}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    className="gap-1.5"
                    onClick={() => {
                      setEditingBackend(null)
                      setBackendDialogOpen(true)
                    }}
                  >
                    <Plus className="size-4" />
                    {t("backends.addBackend")}
                  </Button>
                </div>
              ) : (
                <div className="grid gap-3 md:grid-cols-2">
                  {backends.map((b) => (
                    <div
                      key={b.id}
                      className="flex flex-col justify-between gap-3 rounded-lg border bg-card p-4"
                    >
                      <div className="space-y-2">
                        <div className="flex items-start justify-between gap-2">
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="truncate text-sm font-semibold">{b.name}</span>
                              {!b.enabled ? (
                                <Badge variant="secondary" className="text-[10px]">
                                  {t("backends.statusDisabled")}
                                </Badge>
                              ) : b.status.connected ? (
                                <Badge variant="success" className="text-[10px]">
                                  {t("backends.statusOnline")} · {b.status.transport.toUpperCase()}
                                </Badge>
                              ) : (
                                <Badge variant="destructive" className="text-[10px]">
                                  {t("backends.statusOffline")}
                                </Badge>
                              )}
                            </div>
                            <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                              {b.url}
                            </div>
                          </div>

                          <Switch
                            checked={b.enabled}
                            disabled={toggleBackendMutation.isPending}
                            onCheckedChange={() => toggleBackendMutation.mutate(b)}
                          />
                        </div>

                        <div className="flex flex-wrap items-center gap-2 font-mono text-[11px] text-muted-foreground tabular-nums">
                          {b.status.mihomoVersion && (
                            <Badge variant="outline" className="text-[10px]">
                              {b.status.mihomoVersion}
                            </Badge>
                          )}
                          {b.status.dnsEnhancedMode && (
                            <Badge
                              variant={b.status.dnsEnhancedMode === "fake-ip" ? "info" : "outline"}
                              className="text-[10px]"
                            >
                              DNS: {b.status.dnsEnhancedMode}
                            </Badge>
                          )}
                          {b.status.connected && (
                            <span className="inline-flex items-center gap-2">
                              <span className="inline-flex items-center gap-0.5 text-foreground">
                                <ArrowUp className="size-3 text-chart-1" />
                                {formatSpeed(b.status.uploadSpeed)}
                              </span>
                              <span className="inline-flex items-center gap-0.5 text-foreground">
                                <ArrowDown className="size-3 text-chart-2" />
                                {formatSpeed(b.status.downloadSpeed)}
                              </span>
                            </span>
                          )}
                        </div>

                        {b.enabled && !b.status.connected && b.status.lastError && (
                          <p
                            className="truncate text-xs text-destructive"
                            title={b.status.lastError}
                          >
                            {b.status.lastError}
                          </p>
                        )}
                      </div>

                      <div className="flex items-center justify-end gap-1.5 border-t pt-2.5">
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 gap-1 px-2.5 text-xs"
                          onClick={() => {
                            setEditingBackend(b)
                            setBackendDialogOpen(true)
                          }}
                        >
                          <Pencil className="size-3" />
                          {t("common.edit")}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 px-2.5 text-xs text-destructive hover:bg-destructive-surface hover:text-destructive"
                          onClick={() => {
                            setDeletingBackend(b)
                            setDeleteBackendData(false)
                          }}
                        >
                          <Trash2 className="size-3" />
                          {t("common.delete")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Tab 2: Analytics & Storage Preferences */}
        <TabsContent value="preferences">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("settings.prefsTitle")}</CardTitle>
              <CardDescription className="text-xs">{t("settings.prefsDesc")}</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                onSubmit={prefsForm.handleSubmit((v) => savePrefsMutation.mutate(v))}
                className="space-y-4"
              >
                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <Label htmlFor="pref-grouping" className="text-sm font-medium">
                      {t("settings.domainGroupingLabel")}
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {t("settings.domainGroupingDesc")}
                    </p>
                  </div>
                  <Switch
                    id="pref-grouping"
                    checked={prefsForm.watch("domainGroupingEnabled")}
                    onCheckedChange={(v) => prefsForm.setValue("domainGroupingEnabled", Boolean(v))}
                  />
                </div>

                <Separator />

                <div className="flex items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <Label htmlFor="pref-direct" className="text-sm font-medium">
                      {t("settings.filterDirectLabel")}
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {t("settings.filterDirectDesc")}
                    </p>
                  </div>
                  <Switch
                    id="pref-direct"
                    checked={prefsForm.watch("filterDirectTraffic")}
                    onCheckedChange={(v) => prefsForm.setValue("filterDirectTraffic", Boolean(v))}
                  />
                </div>

                <Separator />

                <div className="flex flex-wrap items-center justify-between gap-4">
                  <div className="space-y-0.5">
                    <Label htmlFor="pref-retention" className="text-sm font-medium">
                      {t("settings.retentionLabel")}
                    </Label>
                    <p className="text-xs text-muted-foreground">{t("settings.retentionDesc")}</p>
                  </div>
                  <div className="w-28 space-y-1">
                    <Input
                      id="pref-retention"
                      type="number"
                      min={1}
                      max={365}
                      className="text-right font-mono"
                      {...prefsForm.register("retentionDays")}
                    />
                    {prefsForm.formState.errors.retentionDays && (
                      <p className="text-xs text-destructive">
                        {prefsForm.formState.errors.retentionDays.message}
                      </p>
                    )}
                  </div>
                </div>

                <div className="flex justify-end pt-2">
                  <Button type="submit" disabled={savePrefsMutation.isPending} className="gap-1.5">
                    {savePrefsMutation.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Save className="size-4" />
                    )}
                    {t("settings.savePrefs")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Tab 3: Data Management (Stats + Vacuum + Custom Date Prune + Clear All) */}
        <TabsContent value="data" className="space-y-4">
          {/* Storage Stats KPI Strip */}
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground">
                  {t("settings.storage.dbSize")}
                </CardTitle>
                <HardDrive className="size-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                {storageQuery.isLoading ? (
                  <Skeleton className="h-7 w-24" />
                ) : (
                  <>
                    <div className="font-mono text-xl font-semibold tabular-nums">
                      {formatBytes(storageStats?.dbSizeBytes ?? 0)}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground tabular-nums">
                      {t("settings.storage.walSize", {
                        bytes: formatBytes(storageStats?.walSizeBytes ?? 0),
                      })}
                    </p>
                  </>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground">
                  {t("settings.storage.detailRows")}
                </CardTitle>
                <Layers className="size-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                {storageQuery.isLoading ? (
                  <Skeleton className="h-7 w-24" />
                ) : (
                  <div className="font-mono text-xl font-semibold tabular-nums">
                    {formatNumber(storageStats?.detailRowCount ?? 0, i18n.language)}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground">
                  {t("settings.storage.summaryRows")}
                </CardTitle>
                <Database className="size-4 text-info" />
              </CardHeader>
              <CardContent>
                {storageQuery.isLoading ? (
                  <Skeleton className="h-7 w-24" />
                ) : (
                  <div className="font-mono text-xl font-semibold tabular-nums">
                    {formatNumber(storageStats?.summaryRowCount ?? 0, i18n.language)}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground">
                  {t("settings.storage.dateSpan")}
                </CardTitle>
                <History className="size-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                {storageQuery.isLoading ? (
                  <Skeleton className="h-7 w-28" />
                ) : (
                  <div className="font-mono text-sm font-semibold tabular-nums">
                    {earliestTS > 0
                      ? t("settings.storage.since", {
                          date: new Intl.DateTimeFormat(i18n.language, {
                            year: "numeric",
                            month: "2-digit",
                            day: "2-digit",
                          }).format(new Date(earliestTS)),
                        })
                      : t("common.noData")}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>

          {/* Vacuum + Custom Date Prune */}
          <div className="grid gap-4 lg:grid-cols-2">
            {/* Reclaim Space (VACUUM) */}
            <Card className="flex flex-col justify-between">
              <CardHeader>
                <CardTitle className="text-base">{t("settings.storage.vacuumTitle")}</CardTitle>
                <CardDescription className="text-xs">
                  {t("settings.storage.vacuumDesc")}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex justify-end">
                <Button
                  variant="outline"
                  disabled={vacuumMutation.isPending}
                  onClick={() => vacuumMutation.mutate()}
                  className="gap-1.5"
                >
                  {vacuumMutation.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Sparkles className="size-4" />
                  )}
                  {vacuumMutation.isPending
                    ? t("settings.storage.vacuumRunning")
                    : t("settings.storage.vacuumButton")}
                </Button>
              </CardContent>
            </Card>

            {/* Custom Date Prune */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t("settings.storage.pruneTitle")}</CardTitle>
                <CardDescription className="text-xs">
                  {t("settings.storage.pruneDesc")}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label className="text-xs">{t("settings.storage.pruneBeforeLabel")}</Label>
                    <Popover open={pruneCalOpen} onOpenChange={setPruneCalOpen}>
                      <PopoverTrigger
                        render={
                          <Button
                            variant="outline"
                            className="h-8 w-full justify-start gap-2 font-mono text-xs font-normal"
                          />
                        }
                      >
                        <CalendarIcon className="size-3.5 text-muted-foreground" />
                        <span>
                          {new Intl.DateTimeFormat(i18n.language, {
                            year: "numeric",
                            month: "2-digit",
                            day: "2-digit",
                          }).format(pruneDate)}
                        </span>
                      </PopoverTrigger>
                      <PopoverContent align="start" className="w-auto p-0">
                        <Calendar
                          mode="single"
                          selected={pruneDate}
                          onSelect={(d) => {
                            if (d) {
                              d.setHours(0, 0, 0, 0)
                              setPruneDate(d)
                              setPruneCalOpen(false)
                            }
                          }}
                          locale={calLocale}
                          disabled={{ after: new Date() }}
                        />
                      </PopoverContent>
                    </Popover>
                  </div>

                  <div className="space-y-1.5">
                    <Label className="text-xs">{t("settings.storage.pruneScopeLabel")}</Label>
                    <Select
                      value={pruneScope}
                      onValueChange={(v) => v && setPruneScope(v as "detail" | "all")}
                    >
                      <SelectTrigger className="h-8 w-full text-xs">
                        <SelectValue>{pruneScopeLabels[pruneScope]}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="detail" className="text-xs">
                          {t("settings.storage.scopeDetail")}
                        </SelectItem>
                        <SelectItem value="all" className="text-xs">
                          {t("settings.storage.scopeAll")}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="flex justify-end">
                  <AlertDialog>
                    <AlertDialogTrigger
                      render={
                        <Button
                          variant="outline"
                          disabled={pruneMutation.isPending}
                          className="gap-1.5"
                        />
                      }
                    >
                      <Eraser className="size-4" />
                      {t("settings.storage.pruneButton")}
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>
                          {t("settings.storage.pruneConfirmTitle", {
                            date: formatDateShort(pruneDate.getTime(), i18n.language),
                          })}
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                          {t("settings.storage.pruneConfirmDesc")}
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                        <AlertDialogAction
                          variant="destructive"
                          onClick={() => pruneMutation.mutate()}
                        >
                          {t("common.confirm")}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Danger Zone: Clear All */}
          <Card className="border-destructive/35">
            <CardHeader>
              <CardTitle className="text-base text-destructive">
                {t("settings.dangerTitle")}
              </CardTitle>
              <CardDescription className="text-xs">{t("settings.dangerDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center justify-between gap-4">
              <div className="text-xs text-muted-foreground">{t("settings.dangerHint")}</div>

              <AlertDialog open={clearDialogOpen} onOpenChange={setClearDialogOpen}>
                <AlertDialogTrigger render={<Button variant="destructive" className="gap-1.5" />}>
                  <Trash2 className="size-4" />
                  {t("settings.clearAllButton")}
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>{t("settings.clearDialogTitle")}</AlertDialogTitle>
                    <AlertDialogDescription>{t("settings.clearDialogDesc")}</AlertDialogDescription>
                  </AlertDialogHeader>
                  <Input
                    value={confirmText}
                    onChange={(e) => setConfirmText(e.target.value)}
                    placeholder={t("settings.clearInputPlaceholder")}
                    className="font-mono"
                  />
                  <AlertDialogFooter>
                    <AlertDialogCancel onClick={() => setConfirmText("")}>
                      {t("common.cancel")}
                    </AlertDialogCancel>
                    <AlertDialogAction
                      disabled={confirmText !== "CLEAR" || clearMutation.isPending}
                      variant="destructive"
                      onClick={(e) => {
                        e.preventDefault()
                        clearMutation.mutate()
                      }}
                    >
                      {clearMutation.isPending
                        ? t("settings.clearing")
                        : t("settings.confirmClear")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Add / Edit Backend Dialog */}
      <BackendFormDialog
        open={backendDialogOpen}
        onOpenChange={setBackendDialogOpen}
        editingBackend={editingBackend}
      />

      {/* Delete Backend Confirmation */}
      <AlertDialog
        open={Boolean(deletingBackend)}
        onOpenChange={(open) => !open && setDeletingBackend(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("backends.deleteTitle", { name: deletingBackend?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>{t("backends.deleteDesc")}</AlertDialogDescription>
          </AlertDialogHeader>
          <label className="flex cursor-pointer items-center gap-2 text-xs">
            <Checkbox
              checked={deleteBackendData}
              onCheckedChange={(v) => setDeleteBackendData(Boolean(v))}
            />
            <span>{t("backends.deleteWithDataLabel")}</span>
          </label>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteBackendMutation.isPending}
              onClick={() =>
                deletingBackend &&
                deleteBackendMutation.mutate({
                  id: deletingBackend.id,
                  deleteData: deleteBackendData,
                })
              }
            >
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
