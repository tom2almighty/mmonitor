import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { ArrowUp } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

export function BackToTopButton() {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const onScroll = () => {
      setVisible(window.scrollY > 300)
    }
    onScroll()
    window.addEventListener("scroll", onScroll, { passive: true })
    return () => window.removeEventListener("scroll", onScroll)
  }, [])

  const scrollToTop = () => {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches
    window.scrollTo({ top: 0, behavior: reduced ? "auto" : "smooth" })
  }

  return (
    <div
      className={cn(
        "fixed right-6 bottom-6 z-40 transition-all duration-200",
        visible ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-3 opacity-0",
      )}
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="outline"
              size="icon"
              aria-label={t("common.backToTop")}
              onClick={scrollToTop}
              className="size-10 rounded-full bg-background/90 shadow-md backdrop-blur hover:bg-accent"
            />
          }
        >
          <ArrowUp className="size-4" />
        </TooltipTrigger>
        <TooltipContent side="left">{t("common.backToTop")}</TooltipContent>
      </Tooltip>
    </div>
  )
}
