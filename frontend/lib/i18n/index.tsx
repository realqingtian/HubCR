"use client";

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { DEFAULT_LOCALE, type Locale, type MessageKey, messages } from "./messages";

// Locale resolution order (per product decision 2026-08-09):
//   1. a previously chosen locale stored in the hubcr_locale cookie (user override)
//   2. the browser's preferred language (navigator.languages)
//   3. the default fallback locale (zh-CN)
//
// SSR cannot read navigator or the browser cookie, so the provider resolves the locale in
// a lazy useState initializer that runs only in the browser. The server renders the
// documented default (zh-CN) and the client applies the resolved locale on first paint.

const LOCALE_COOKIE = "hubcr_locale";
const LOCALE_STORAGE_DAYS = 365;

const SUPPORTED: Locale[] = ["zh", "en"];

function isLocale(value: string | undefined | null): value is Locale {
  return value !== undefined && value !== null && (SUPPORTED as string[]).includes(value);
}

// matchBrowser maps an Accept-Language / navigator style tag to a supported Locale.
// "zh-CN", "zh-Hans", "zh" -> zh; "en-US", "en" -> en; anything else -> undefined.
export function matchBrowserLanguage(tag: string | undefined): Locale | undefined {
  if (!tag) return undefined;
  const lower = tag.toLowerCase();
  if (lower.startsWith("zh")) return "zh";
  if (lower.startsWith("en")) return "en";
  return undefined;
}

function readCookieLocale(): Locale | undefined {
  if (typeof document === "undefined") return undefined;
  const match = document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${LOCALE_COOKIE}=`));
  const value = match?.split("=")[1];
  return isLocale(value) ? value : undefined;
}

function browserLanguages(): readonly string[] | undefined {
  return typeof navigator !== "undefined" ? Array.from(navigator.languages) : undefined;
}

export function persistLocale(locale: Locale): void {
  if (typeof document === "undefined") return;
  const expires = new Date(Date.now() + LOCALE_STORAGE_DAYS * 24 * 60 * 60 * 1000).toUTCString();
  document.cookie = `${LOCALE_COOKIE}=${locale}; expires=${expires}; path=/; samesite=lax`;
}

// resolveLocale is exported for unit tests so the precedence rules can be verified without
// a DOM. The inputs mirror what the provider gathers from the environment.
export function resolveLocale(
  cookie: string | undefined,
  browserLanguages: readonly string[] | undefined,
): Locale {
  const fromCookie = matchBrowserLanguage(cookie);
  if (fromCookie) return fromCookie;
  if (browserLanguages) {
    for (const tag of browserLanguages) {
      const match = matchBrowserLanguage(tag);
      if (match) return match;
    }
  }
  return DEFAULT_LOCALE;
}

type Translate = (key: MessageKey, params?: Record<string, string | number>) => string;

interface LocaleContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: Translate;
}

const LocaleContext = createContext<LocaleContextValue | null>(null);

function formatMessage(template: string, params?: Record<string, string | number>): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_match, name: string) =>
    params[name] !== undefined ? String(params[name]) : `{${name}}`,
  );
}

export function LocaleProvider({ children }: Readonly<{ children: ReactNode }>) {
  // Resolve once on the client using a lazy initializer. SSR renders the default locale
  // (the initializer reads navigator/cookie only in the browser). This avoids a setState
  // side-effect after mount while still applying the cookie/browser/default precedence.
  const [locale, setLocaleState] = useState<Locale>(() => {
    if (typeof document === "undefined") return DEFAULT_LOCALE;
    return resolveLocale(readCookieLocale(), browserLanguages());
  });

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    persistLocale(next);
  }, []);

  const value = useMemo<LocaleContextValue>(() => {
    const catalog = messages[locale];
    const translate: Translate = (key, params) => {
      const entry = catalog[key] ?? messages[DEFAULT_LOCALE][key] ?? key;
      return formatMessage(entry, params);
    };
    return { locale, setLocale, t: translate };
  }, [locale, setLocale]);

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocale(): LocaleContextValue {
  const context = useContext(LocaleContext);
  if (context === null) {
    throw new Error("useLocale must be used within a LocaleProvider");
  }
  return context;
}

// useT returns only the translate function, for components that do not need to read or
// change the locale.
export function useT(): Translate {
  return useLocale().t;
}
