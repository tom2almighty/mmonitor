import { useTranslation } from "react-i18next"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

interface DnsModeBadgeProps {
  mode: string
  className?: string
}

export function DnsModeBadge({ mode, className }: DnsModeBadgeProps) {
  const { t } = useTranslation()

  const label =
    mode === "fake-ip"
      ? t("dnsMode.fake-ip")
      : mode === "fake-ip-unresolved"
        ? t("dnsMode.fake-ip-unresolved")
        : mode === "redir-host"
          ? t("dnsMode.redir-host")
          : mode === "normal"
            ? t("dnsMode.normal")
            : mode || t("dnsMode.unknown")

  const variant =
    mode === "fake-ip"
      ? "info"
      : mode === "fake-ip-unresolved"
        ? "warning"
        : mode === "redir-host"
          ? "outline"
          : "secondary"

  return (
    <Badge variant={variant} className={cn("font-mono text-[11px]", className)}>
      {label}
    </Badge>
  )
}
