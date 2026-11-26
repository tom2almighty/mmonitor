import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { ThemeProvider } from "@/hooks/use-theme"
import { FilterProvider } from "@/hooks/use-filter-context"
import { AppShell } from "@/components/layout/AppShell"
import { OverviewPage } from "@/pages/OverviewPage"
import { ExplorerPage } from "@/pages/ExplorerPage"
import { LiveConnectionsPage } from "@/pages/LiveConnectionsPage"
import { SettingsPage } from "@/pages/SettingsPage"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider delay={200}>
          <FilterProvider>
            <BrowserRouter>
              <AppShell>
                <Routes>
                  <Route path="/" element={<OverviewPage />} />
                  <Route path="/explorer" element={<ExplorerPage />} />
                  <Route path="/connections" element={<LiveConnectionsPage />} />
                  <Route path="/settings" element={<SettingsPage />} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Routes>
              </AppShell>
            </BrowserRouter>
            <Toaster position="bottom-right" richColors />
          </FilterProvider>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

export default App
