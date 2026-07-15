import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { useState, type ReactNode } from "react";
import { Toaster } from "sonner";

import { TooltipProvider } from "@/components/ui/tooltip";

export function PublicProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 15_000, refetchOnWindowFocus: false }, mutations: { retry: 0 } } }));
  return <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange><QueryClientProvider client={queryClient}><TooltipProvider delayDuration={300}>{children}<Toaster richColors closeButton position="top-right" /></TooltipProvider></QueryClientProvider></ThemeProvider>;
}
