"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { LocaleProvider, useLocale } from "@/lib/i18n";

export function AppProviders({ children }: Readonly<{ children: React.ReactNode }>) {
  const [queryClient] = useState(() => new QueryClient());

  return (
    <QueryClientProvider client={queryClient}>
      <LocaleProvider>
        <HtmlLangSync />
        {children}
      </LocaleProvider>
    </QueryClientProvider>
  );
}

// HtmlLangSync keeps the <html lang> attribute in sync with the resolved locale so screen
// readers and browser heuristics match the active language. The initial server-rendered
// value is the default locale (zh-CN).
function HtmlLangSync() {
  const { locale } = useLocale();
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return null;
}
