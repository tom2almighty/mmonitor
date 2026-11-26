import { useEffect, useMemo, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import type { TFunction } from "i18next"
import { CheckCircle2, Loader2, PlugZap, Save, XCircle } from "lucide-react"
import { toast } from "sonner"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Badge } from "@/components/ui/badge"
import { api } from "@/lib/api"
import type { BackendWithStatus, TestConnectionResult } from "@/lib/types"

const createSchema = (t: TFunction) =>
  z.object({
    name: z.string().trim().min(1, t("backends.validation.nameRequired")),
    url: z.string().trim().url(t("settings.validation.invalidUrl")),
    secret: z.string().optional(),
    clearSecret: z.boolean(),
    enabled: z.boolean(),
  })

type FormValues = {
  name: string
  url: string
  secret?: string
  clearSecret: boolean
  enabled: boolean
}

interface BackendFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingBackend?: BackendWithStatus | null
}

export function BackendFormDialog({ open, onOpenChange, editingBackend }: BackendFormDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [testResult, setTestResult] = useState<TestConnectionResult | null>(null)
  const schema = useMemo(() => createSchema(t), [t])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: "",
      url: "http://127.0.0.1:9090",
      secret: "",
      clearSecret: false,
      enabled: true,
    },
  })

  useEffect(() => {
    if (!open) return
    setTestResult(null)
    if (editingBackend) {
      form.reset({
        name: editingBackend.name,
        url: editingBackend.url,
        secret: "",
        clearSecret: false,
        enabled: editingBackend.enabled,
      })
    } else {
      form.reset({
        name: "",
        url: "http://127.0.0.1:9090",
        secret: "",
        clearSecret: false,
        enabled: true,
      })
    }
  }, [open, editingBackend, form])

  const testMutation = useMutation({
    mutationFn: () => {
      const vals = form.getValues()
      return api.testBackend({
        backendId: editingBackend?.id,
        url: vals.url,
        secret: vals.secret || undefined,
        clearSecret: vals.clearSecret,
      })
    },
    onSuccess: (res) => {
      setTestResult(res)
      if (res.ok) {
        toast.success(t("settings.testOkToast", { version: res.version || "" }))
      } else {
        toast.error(res.error || t("settings.testFail", { error: "" }))
      }
    },
    onError: (err: Error) => toast.error(err.message),
  })

  const saveMutation = useMutation({
    mutationFn: (values: FormValues) => {
      const payload = {
        name: values.name,
        url: values.url,
        secret: values.secret || undefined,
        clearSecret: values.clearSecret,
        enabled: values.enabled,
      }
      return editingBackend
        ? api.updateBackend(editingBackend.id, payload)
        : api.createBackend(payload)
    },
    onSuccess: () => {
      toast.success(editingBackend ? t("backends.updatedToast") : t("backends.createdToast"))
      void queryClient.invalidateQueries({ queryKey: ["backends"] })
      void queryClient.invalidateQueries({ queryKey: ["status"] })
      onOpenChange(false)
    },
    onError: (err: Error) => toast.error(err.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editingBackend ? t("backends.editTitle") : t("backends.addTitle")}
          </DialogTitle>
          <DialogDescription>{t("backends.dialogDesc")}</DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit((v) => saveMutation.mutate(v))} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="backend-name" className="text-xs">
              {t("backends.nameLabel")}
            </Label>
            <Input
              id="backend-name"
              placeholder={t("backends.namePlaceholder")}
              {...form.register("name")}
            />
            {form.formState.errors.name && (
              <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="backend-url" className="text-xs">
              {t("settings.urlLabel")}
            </Label>
            <Input
              id="backend-url"
              placeholder="http://127.0.0.1:9090"
              className="font-mono text-sm"
              {...form.register("url")}
            />
            {form.formState.errors.url && (
              <p className="text-xs text-destructive">{form.formState.errors.url.message}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="backend-secret" className="text-xs">
                {t("settings.secretLabel")}
              </Label>
              {editingBackend?.hasSecret && (
                <Badge variant="info" className="h-4 px-1.5 text-[10px]">
                  {t("settings.secretSavedBadge")}
                </Badge>
              )}
            </div>
            <Input
              id="backend-secret"
              type="password"
              autoComplete="off"
              placeholder={
                editingBackend?.hasSecret
                  ? t("settings.secretPlaceholderSaved")
                  : t("settings.secretPlaceholderEmpty")
              }
              {...form.register("secret")}
            />
            {editingBackend?.hasSecret && (
              <div className="flex items-center justify-between pt-1">
                <span className="text-xs text-muted-foreground">
                  {t("settings.clearSecretLabel")}
                </span>
                <Switch
                  checked={form.watch("clearSecret")}
                  onCheckedChange={(v) => form.setValue("clearSecret", Boolean(v))}
                />
              </div>
            )}
          </div>

          <div className="flex items-center justify-between rounded-lg border px-3 py-2.5">
            <div className="space-y-0.5">
              <Label htmlFor="backend-enabled" className="text-xs font-medium">
                {t("backends.enabledLabel")}
              </Label>
              <p className="text-[11px] text-muted-foreground">{t("backends.enabledDesc")}</p>
            </div>
            <Switch
              id="backend-enabled"
              checked={form.watch("enabled")}
              onCheckedChange={(v) => form.setValue("enabled", Boolean(v))}
            />
          </div>

          {testResult && (
            <div
              className={
                testResult.ok
                  ? "rounded-lg border border-success/30 bg-success-surface p-3 text-xs"
                  : "rounded-lg border border-destructive/30 bg-destructive-surface p-3 text-xs"
              }
            >
              <div className="flex items-center gap-2 font-medium">
                {testResult.ok ? (
                  <CheckCircle2 className="size-4 text-success" />
                ) : (
                  <XCircle className="size-4 text-destructive" />
                )}
                <span className={testResult.ok ? "text-success" : "text-destructive"}>
                  {testResult.ok
                    ? t("settings.testOk", { ms: testResult.latencyMs })
                    : t("settings.testFail", { error: testResult.error ?? "" })}
                </span>
              </div>
              {testResult.ok && (
                <div className="mt-1.5 flex flex-wrap gap-3 font-mono text-[11px] text-muted-foreground">
                  {testResult.version && (
                    <span>{t("settings.metaVersion", { value: testResult.version })}</span>
                  )}
                  {testResult.mode && (
                    <span>{t("settings.metaMode", { value: testResult.mode })}</span>
                  )}
                  {testResult.dnsEnhancedMode && (
                    <span>{t("settings.metaDns", { value: testResult.dnsEnhancedMode })}</span>
                  )}
                  {testResult.fakeIPRange && (
                    <span>{t("settings.metaFakeIPRange", { value: testResult.fakeIPRange })}</span>
                  )}
                </div>
              )}
            </div>
          )}

          <DialogFooter className="gap-2 sm:justify-between">
            <Button
              type="button"
              variant="outline"
              disabled={testMutation.isPending}
              onClick={() => testMutation.mutate()}
              className="gap-1.5"
            >
              {testMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <PlugZap className="size-4" />
              )}
              {t("settings.testButton")}
            </Button>
            <div className="flex items-center gap-2">
              <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saveMutation.isPending} className="gap-1.5">
                {saveMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Save className="size-4" />
                )}
                {t("common.save")}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
