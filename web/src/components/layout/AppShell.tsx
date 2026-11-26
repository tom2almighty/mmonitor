import { useState, type ReactNode } from "react"
import { NavLink, useLocation, useNavigate } from "react-router-dom"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  BarChart3,
  Check,
  ChevronsUpDown,
  Languages,
  LayoutDashboard,
  Menu,
  Monitor,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Radio,
  RefreshCw,
  Server,
  Settings,
  Sun,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Sheet, SheetContent } from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { DateRangePicker } from "@/components/common/DateRangePicker"
import { BackToTopButton } from "@/components/common/BackToTopButton"
import { BackendFormDialog } from "@/components/common/BackendFormDialog"
import { api } from "@/lib/api"
import { formatSpeed } from "@/lib/format"
import { useTheme } from "@/hooks/use-theme"
import { useFilters } from "@/hooks/use-filter-context"
import { cn } from "@/lib/utils"
import type { DNSModeFilter, TimeRangePreset } from "@/lib/types"

export function AppShell({ children }: { children: ReactNode }) {
  const { t, i18n } = useTranslation()
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const [addBackendOpen, setAddBackendOpen] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { theme, setTheme } = useTheme()
  const {
    backendId,
    setBackendId,
    preset,
    setPreset,
    mode,
    setMode,
    dnsMode,
    setDNSMode,
    start,
    end,
    setCustomRange,
    triggerRefresh,
  } = useFilters()

  const backendsQuery = useQuery({
    queryKey: ["backends"],
    queryFn: api.listBackends,
    refetchInterval: 3000,
  })

  const statusQuery = useQuery({
    queryKey: ["status", backendId],
    queryFn: () => api.getStatus(backendId),
    refetchInterval: 2000,
  })

  const navItems = [
    { to: "/", label: t("nav.overview"), icon: LayoutDashboard, end: true },
    { to: "/explorer", label: t("nav.explorer"), icon: BarChart3 },
    { to: "/connections", label: t("nav.connections"), icon: Radio },
    { to: "/settings", label: t("nav.settings"), icon: Settings },
  ]

  const routeTitles: Record<string, string> = {
    "/": t("nav.overview"),
    "/explorer": t("nav.explorer"),
    "/connections": t("nav.connections"),
    "/settings": t("nav.settings"),
  }

  const rangeOptions: Array<{ value: TimeRangePreset; label: string }> = [
    { value: "today", label: t("filters.today") },
    { value: "1d", label: t("filters.last1d") },
    { value: "3d", label: t("filters.last3d") },
    { value: "7d", label: t("filters.last7d") },
    { value: "15d", label: t("filters.last15d") },
    { value: "30d", label: t("filters.last30d") },
    ...(mode === "summary"
      ? [{ value: "all" as TimeRangePreset, label: t("filters.allHistory") }]
      : []),
    { value: "custom", label: t("filters.custom") },
  ]

  const dnsOptions: Array<{ value: DNSModeFilter; label: string }> = [
    { value: "all", label: t("filters.dnsAll") },
    { value: "fake-ip", label: t("filters.dnsFakeIP") },
    { value: "fake-ip-unresolved", label: t("filters.dnsFakeIPUnresolved") },
    { value: "normal", label: t("filters.dnsNormal") },
    { value: "redir-host", label: t("filters.dnsRedirHost") },
  ]

  const rangeLabelMap: Record<string, string> = Object.fromEntries(
    rangeOptions.map((o) => [o.value, o.label]),
  )
  const dnsLabelMap: Record<string, string> = Object.fromEntries(
    dnsOptions.map((o) => [o.value, o.label]),
  )

  const backends = backendsQuery.data ?? []
  const status = statusQuery.data
  const activeBackend = backendId > 0 ? backends.find((b) => b.id === backendId) : null
  const showTimeFilters = location.pathname === "/" || location.pathname === "/explorer"
  const pageTitle = routeTitles[location.pathname] ?? t("brand.name")
  const currentLang = i18n.language.startsWith("zh") ? "zh-CN" : "en-US"

  const handleRefresh = () => {
    triggerRefresh()
    void queryClient.invalidateQueries()
  }

  const backendSwitcherMenu = (isCompact: boolean) => (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          isCompact ? (
            <button
              type="button"
              aria-label={t("backends.switchBackend")}
              className="relative flex size-9 items-center justify-center rounded-lg border border-sidebar-border bg-background/70 text-sidebar-foreground transition-colors hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          ) : (
            <button
              type="button"
              className="flex w-full flex-col gap-1.5 rounded-lg border border-sidebar-border bg-background/70 p-2.5 text-left text-xs transition-colors hover:bg-sidebar-accent/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          )
        }
      >
        {isCompact ? (
          <>
            <Server className="size-4" />
            <span
              className={cn(
                "absolute top-1.5 right-1.5 size-2 rounded-full ring-2 ring-sidebar",
                status?.connected
                  ? "bg-success"
                  : backends.length > 0
                    ? "bg-destructive"
                    : "bg-warning",
              )}
            />
          </>
        ) : (
          <>
            <div className="flex w-full items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-1.5">
                <span
                  className={cn(
                    "size-2 shrink-0 rounded-full",
                    status?.connected
                      ? "bg-success"
                      : backends.length > 0
                        ? "bg-destructive"
                        : "bg-warning",
                  )}
                />
                <span className="truncate font-medium">
                  {activeBackend
                    ? activeBackend.name
                    : backends.length > 0
                      ? t("backends.allBackends")
                      : t("backends.noBackends")}
                </span>
              </div>
              <ChevronsUpDown className="size-3.5 shrink-0 text-muted-foreground" />
            </div>

            {status?.connected ? (
              <div className="grid w-full grid-cols-2 gap-1 font-mono text-[11px] tabular-nums">
                <div className="flex items-center gap-1 text-muted-foreground">
                  <ArrowUp className="size-3 text-chart-1" />
                  <span className="truncate text-foreground">
                    {formatSpeed(status.uploadSpeed)}
                  </span>
                </div>
                <div className="flex items-center gap-1 text-muted-foreground">
                  <ArrowDown className="size-3 text-chart-2" />
                  <span className="truncate text-foreground">
                    {formatSpeed(status.downloadSpeed)}
                  </span>
                </div>
              </div>
            ) : (
              <div className="text-[11px] text-muted-foreground">
                {backends.length > 0 ? t("status.offline") : t("backends.clickToAdd")}
              </div>
            )}
          </>
        )}
      </DropdownMenuTrigger>

      <DropdownMenuContent side="top" align="start" className="w-60">
        {backends.length > 1 && (
          <DropdownMenuItem onClick={() => setBackendId(0)}>
            <Server className="size-3.5 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs font-medium">{t("backends.allBackends")}</div>
              <div className="text-[10px] text-muted-foreground tabular-nums">
                {t("backends.onlineRatio", {
                  online: backends.filter((b) => b.status.connected).length,
                  total: backends.length,
                })}
              </div>
            </div>
            {backendId === 0 && <Check className="size-3.5" />}
          </DropdownMenuItem>
        )}

        {backends.map((b) => (
          <DropdownMenuItem key={b.id} onClick={() => setBackendId(b.id)}>
            <span
              className={cn(
                "size-2 shrink-0 rounded-full",
                b.status.connected
                  ? "bg-success"
                  : b.enabled
                    ? "bg-destructive"
                    : "bg-muted-foreground",
              )}
            />
            <div className="min-w-0 flex-1">
              <div className="truncate text-xs font-medium">{b.name}</div>
              <div className="truncate font-mono text-[10px] text-muted-foreground">
                {b.status.connected
                  ? `↑ ${formatSpeed(b.status.uploadSpeed)} · ↓ ${formatSpeed(b.status.downloadSpeed)}`
                  : b.url}
              </div>
            </div>
            {backendId === b.id && <Check className="size-3.5" />}
          </DropdownMenuItem>
        ))}

        {backends.length > 0 && <DropdownMenuSeparator />}

        <DropdownMenuItem onClick={() => setAddBackendOpen(true)}>
          <Plus className="size-3.5" />
          <span className="text-xs">{t("backends.addBackend")}</span>
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            setMobileOpen(false)
            navigate("/settings?tab=backends")
          }}
        >
          <Settings className="size-3.5" />
          <span className="text-xs">{t("backends.manageBackends")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )

  const footerQuickActions = (isCompact: boolean) => (
    <div
      className={cn("flex items-center", isCompact ? "flex-col gap-1.5" : "justify-between gap-1")}
    >
      {/* Refresh */}
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon"
              className={cn(
                "text-muted-foreground hover:text-foreground",
                isCompact ? "size-9" : "h-8 flex-1",
              )}
              aria-label={t("common.refresh")}
              onClick={handleRefresh}
            />
          }
        >
          <RefreshCw className="size-3.5" />
        </TooltipTrigger>
        <TooltipContent side={isCompact ? "right" : "top"}>{t("common.refresh")}</TooltipContent>
      </Tooltip>

      {/* Language */}
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger
            render={
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className={cn(
                      "text-muted-foreground hover:text-foreground",
                      isCompact ? "size-9" : "h-8 flex-1",
                    )}
                    aria-label={t("language.label")}
                  />
                }
              />
            }
          >
            <Languages className="size-3.5" />
          </TooltipTrigger>
          <TooltipContent side={isCompact ? "right" : "top"}>{t("language.label")}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent side={isCompact ? "right" : "top"} align="center" className="w-32">
          <DropdownMenuItem onClick={() => void i18n.changeLanguage("zh-CN")}>
            <span className="flex-1">{t("language.zhCN")}</span>
            {currentLang === "zh-CN" && <Check className="size-3.5" />}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => void i18n.changeLanguage("en-US")}>
            <span className="flex-1">{t("language.enUS")}</span>
            {currentLang === "en-US" && <Check className="size-3.5" />}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Theme */}
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger
            render={
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className={cn(
                      "text-muted-foreground hover:text-foreground",
                      isCompact ? "size-9" : "h-8 flex-1",
                    )}
                    aria-label={t("theme.label")}
                  />
                }
              />
            }
          >
            {theme === "dark" ? (
              <Moon className="size-3.5" />
            ) : theme === "light" ? (
              <Sun className="size-3.5" />
            ) : (
              <Monitor className="size-3.5" />
            )}
          </TooltipTrigger>
          <TooltipContent side={isCompact ? "right" : "top"}>{t("theme.label")}</TooltipContent>
        </Tooltip>
        <DropdownMenuContent side={isCompact ? "right" : "top"} align="end" className="w-36">
          <DropdownMenuItem onClick={() => setTheme("light")}>
            <Sun className="size-3.5" />
            <span className="flex-1">{t("theme.light")}</span>
            {theme === "light" && <Check className="size-3.5" />}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setTheme("dark")}>
            <Moon className="size-3.5" />
            <span className="flex-1">{t("theme.dark")}</span>
            {theme === "dark" && <Check className="size-3.5" />}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setTheme("system")}>
            <Monitor className="size-3.5" />
            <span className="flex-1">{t("theme.system")}</span>
            {theme === "system" && <Check className="size-3.5" />}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )

  const sidebarContent = (isCompact: boolean) => (
    <div className="flex h-full flex-col bg-sidebar text-sidebar-foreground">
      {/* Top Brand + Fixed-Position Collapse/Expand */}
      <div
        className={cn(
          "flex h-14 shrink-0 items-center border-b border-sidebar-border",
          isCompact ? "justify-center px-0" : "justify-between px-3",
        )}
      >
        {isCompact ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  aria-label={t("nav.expandSidebar")}
                  onClick={() => setCollapsed(false)}
                  className="group/expand relative flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              }
            >
              <Activity className="size-4 transition-opacity duration-150 group-hover/expand:opacity-0 group-focus-visible/expand:opacity-0" />
              <PanelLeftOpen className="absolute size-4 opacity-0 transition-opacity duration-150 group-hover/expand:opacity-100 group-focus-visible/expand:opacity-100" />
            </TooltipTrigger>
            <TooltipContent side="right">{t("nav.expandSidebar")}</TooltipContent>
          </Tooltip>
        ) : (
          <>
            <div className="flex items-center gap-2.5 overflow-hidden">
              <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                <Activity className="size-4" />
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold tracking-tight">
                  {t("brand.name")}
                </div>
                <div className="truncate text-[11px] text-muted-foreground">
                  {t("brand.subtitle")}
                </div>
              </div>
            </div>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="hidden size-7 shrink-0 md:inline-flex"
                    aria-label={t("nav.collapseSidebar")}
                    onClick={() => setCollapsed(true)}
                  />
                }
              >
                <PanelLeftClose className="size-4" />
              </TooltipTrigger>
              <TooltipContent side="right">{t("nav.collapseSidebar")}</TooltipContent>
            </Tooltip>
          </>
        )}
      </div>

      {/* Navigation */}
      <nav
        className={cn(
          "flex flex-1 flex-col gap-1 overflow-y-auto py-2",
          isCompact ? "items-center px-0" : "px-2",
        )}
      >
        {navItems.map((item) => {
          const Icon = item.icon

          if (isCompact) {
            return (
              <Tooltip key={item.to}>
                <TooltipTrigger
                  render={
                    <NavLink
                      to={item.to}
                      end={item.end}
                      aria-label={item.label}
                      className={({ isActive }) =>
                        cn(
                          "flex size-9 items-center justify-center rounded-lg transition-colors",
                          isActive
                            ? "bg-sidebar-accent text-sidebar-accent-foreground"
                            : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
                        )
                      }
                    />
                  }
                >
                  <Icon className="size-4 shrink-0" />
                </TooltipTrigger>
                <TooltipContent side="right">{item.label}</TooltipContent>
              </Tooltip>
            )
          }

          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={() => setMobileOpen(false)}
              className={({ isActive }) =>
                cn(
                  "flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-sidebar-accent text-sidebar-accent-foreground"
                    : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
                )
              }
            >
              <Icon className="size-4 shrink-0" />
              <span className="truncate">{item.label}</span>
              {item.to === "/connections" && (status?.activeConnections ?? 0) > 0 && (
                <Badge
                  variant="secondary"
                  className="ml-auto h-5 px-1.5 font-mono text-[10px] tabular-nums"
                >
                  {status?.activeConnections}
                </Badge>
              )}
            </NavLink>
          )
        })}
      </nav>

      {/* Sidebar Footer: Multi-Backend Switcher + Refresh / Language / Theme */}
      <div
        className={cn(
          "shrink-0 space-y-2 border-t border-sidebar-border",
          isCompact ? "flex flex-col items-center px-0 py-2.5" : "p-2.5",
        )}
      >
        {backendSwitcherMenu(isCompact)}
        {footerQuickActions(isCompact)}
      </div>
    </div>
  )

  return (
    <div className="min-h-screen w-full bg-background text-foreground">
      {/* Fixed Desktop Sidebar — never scrolls with main page */}
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-30 hidden border-r border-sidebar-border transition-[width] duration-200 ease-out md:block",
          collapsed ? "w-14" : "w-56",
        )}
      >
        {sidebarContent(collapsed)}
      </aside>

      {/* Mobile Drawer Sidebar */}
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-60 p-0" showCloseButton={false}>
          {sidebarContent(false)}
        </SheetContent>
      </Sheet>

      {/* Main Column offset by fixed sidebar width */}
      <div
        className={cn(
          "flex min-h-screen min-w-0 flex-col transition-[padding] duration-200 ease-out",
          collapsed ? "md:pl-14" : "md:pl-56",
        )}
      >
        {/* Top Filter Bar */}
        <header className="sticky top-0 z-20 flex min-h-14 flex-wrap items-center justify-between gap-2 border-b bg-background/95 px-4 py-2 backdrop-blur supports-backdrop-filter:bg-background/80 lg:px-6">
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              className="size-8 md:hidden"
              aria-label={t("nav.openMenu")}
              onClick={() => setMobileOpen(true)}
            >
              <Menu className="size-4" />
            </Button>
            <h1 className="text-base font-semibold tracking-tight">{pageTitle}</h1>
            {activeBackend && (
              <Badge variant="outline" className="hidden font-mono text-[11px] sm:inline-flex">
                {activeBackend.name}
              </Badge>
            )}
          </div>

          {showTimeFilters && (
            <div className="flex flex-wrap items-center gap-2">
              {/* Detail / Summary segmented switch */}
              <div className="flex items-center rounded-lg border bg-muted/40 p-0.5">
                <Button
                  type="button"
                  size="sm"
                  variant={mode === "detail" ? "secondary" : "ghost"}
                  className="h-7 px-2.5 text-xs"
                  onClick={() => setMode("detail")}
                >
                  {t("filters.detail")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={mode === "summary" ? "secondary" : "ghost"}
                  className="h-7 px-2.5 text-xs"
                  onClick={() => setMode("summary")}
                >
                  {t("filters.summary")}
                </Button>
              </div>

              {/* Time range preset */}
              <Select value={preset} onValueChange={(v) => v && setPreset(v as TimeRangePreset)}>
                <SelectTrigger className="h-8 min-w-32 text-xs">
                  <SelectValue>{rangeLabelMap[preset] ?? t("filters.last7d")}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {rangeOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value} className="text-xs">
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              {/* Shadcn Calendar Date Range Picker */}
              {preset === "custom" && (
                <DateRangePicker
                  start={start}
                  end={end}
                  onChange={(s, e) => setCustomRange(s, e)}
                />
              )}

              {/* DNS Mode Filter */}
              <Select value={dnsMode} onValueChange={(v) => v && setDNSMode(v as DNSModeFilter)}>
                <SelectTrigger className="h-8 min-w-36 text-xs">
                  <SelectValue>{dnsLabelMap[dnsMode]}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {dnsOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value} className="text-xs">
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </header>

        {/* Unconfigured Warning Banner */}
        {backendsQuery.isSuccess && backends.length === 0 && location.pathname !== "/settings" && (
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-warning/30 bg-warning-surface px-4 py-2.5 text-xs lg:px-6">
            <div className="flex items-center gap-2 font-medium text-warning">
              <AlertTriangle className="size-4 shrink-0" />
              <span>{t("status.bannerUnconfigured")}</span>
            </div>
            <Button size="sm" className="h-7 text-xs" onClick={() => setAddBackendOpen(true)}>
              {t("backends.addBackend")}
            </Button>
          </div>
        )}

        {/* Page Body */}
        <main className="mx-auto w-full max-w-360 flex-1 p-4 lg:p-6">{children}</main>
      </div>

      <BackToTopButton />
      <BackendFormDialog open={addBackendOpen} onOpenChange={setAddBackendOpen} />
    </div>
  )
}
