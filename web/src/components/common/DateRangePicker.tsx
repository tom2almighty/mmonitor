import { useState } from "react"
import { useTranslation } from "react-i18next"
import { CalendarIcon } from "lucide-react"
import type { DateRange } from "react-day-picker"
import { enUS, zhCN } from "date-fns/locale"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { formatDateShort } from "@/lib/format"
import { cn } from "@/lib/utils"

interface DateRangePickerProps {
  start: number
  end: number
  onChange: (start: number, end: number) => void
  className?: string
}

export function DateRangePicker({ start, end, onChange, className }: DateRangePickerProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)

  const selected: DateRange = {
    from: new Date(start),
    to: new Date(end),
  }

  const locale = i18n.language.startsWith("zh") ? zhCN : enUS

  const handleSelect = (range: DateRange | undefined) => {
    if (!range?.from) return
    const fromDate = new Date(
      range.from.getFullYear(),
      range.from.getMonth(),
      range.from.getDate(),
      0,
      0,
      0,
      0,
    )
    const toBase = range.to ?? range.from
    const toDate = new Date(
      toBase.getFullYear(),
      toBase.getMonth(),
      toBase.getDate(),
      23,
      59,
      59,
      999,
    )
    onChange(fromDate.getTime(), toDate.getTime())
    if (range.from && range.to && range.to.getTime() !== range.from.getTime()) {
      setOpen(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className={cn(
              "h-8 justify-start gap-1.5 font-mono text-xs font-normal tabular-nums",
              className,
            )}
          />
        }
      >
        <CalendarIcon className="size-3.5 text-muted-foreground" />
        <span>
          {start && end
            ? `${formatDateShort(start, i18n.language)} – ${formatDateShort(end, i18n.language)}`
            : t("filters.selectDateRange")}
        </span>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-auto p-0">
        <Calendar
          mode="range"
          defaultMonth={selected.from}
          selected={selected}
          onSelect={handleSelect}
          numberOfMonths={2}
          locale={locale}
          disabled={{ after: new Date() }}
        />
      </PopoverContent>
    </Popover>
  )
}
