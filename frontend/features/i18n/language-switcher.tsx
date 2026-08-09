"use client";

import { LOCALE_LABELS } from "@/lib/i18n/messages";
import { useLocale } from "@/lib/i18n";

// LanguageSwitcher renders a compact control for choosing between the supported locales.
// The choice is persisted to the hubcr_locale cookie via setLocale, so it survives reloads
// and overrides browser-language detection on the next visit.
export function LanguageSwitcher() {
  const { locale, setLocale } = useLocale();
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-slate-300 bg-white p-0.5">
      {(Object.keys(LOCALE_LABELS) as Array<keyof typeof LOCALE_LABELS>).map((code) => {
        const active = code === locale;
        return (
          <button
            aria-pressed={active}
            className={`rounded-full px-2.5 py-1 text-xs font-semibold focus:outline-none focus:ring-4 focus:ring-sky-100 ${active ? "bg-slate-950 text-white" : "text-slate-600 hover:bg-slate-100"}`}
            key={code}
            lang={code}
            onClick={() => setLocale(code)}
            type="button"
          >
            {LOCALE_LABELS[code]}
          </button>
        );
      })}
    </span>
  );
}
