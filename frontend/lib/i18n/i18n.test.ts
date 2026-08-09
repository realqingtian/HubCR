import { describe, expect, it } from "vitest";
import { matchBrowserLanguage, resolveLocale } from "./index";
import { DEFAULT_LOCALE, messages, type MessageKey } from "./messages";

describe("matchBrowserLanguage", () => {
  it("maps Chinese variants to zh", () => {
    expect(matchBrowserLanguage("zh-CN")).toBe("zh");
    expect(matchBrowserLanguage("zh-Hans")).toBe("zh");
    expect(matchBrowserLanguage("zh")).toBe("zh");
    expect(matchBrowserLanguage("zh-TW")).toBe("zh");
  });

  it("maps English variants to en", () => {
    expect(matchBrowserLanguage("en-US")).toBe("en");
    expect(matchBrowserLanguage("en-GB")).toBe("en");
    expect(matchBrowserLanguage("en")).toBe("en");
  });

  it("returns undefined for unsupported or empty input", () => {
    expect(matchBrowserLanguage("fr-FR")).toBeUndefined();
    expect(matchBrowserLanguage("ja")).toBeUndefined();
    expect(matchBrowserLanguage(undefined)).toBeUndefined();
    expect(matchBrowserLanguage("")).toBeUndefined();
  });
});

describe("resolveLocale precedence", () => {
  it("prefers the cookie over the browser language", () => {
    expect(resolveLocale("en", ["zh-CN", "en-US"])).toBe("en");
    expect(resolveLocale("zh", ["en-US"])).toBe("zh");
  });

  it("ignores an unsupported cookie value and falls back to the browser", () => {
    expect(resolveLocale("fr", ["en-US"])).toBe("en");
    expect(resolveLocale("unsupported", ["zh-CN"])).toBe("zh");
  });

  it("uses the first matching browser language when no cookie is set", () => {
    expect(resolveLocale(undefined, ["fr-FR", "en-US", "zh-CN"])).toBe("en");
    expect(resolveLocale(undefined, ["zh-Hans"])).toBe("zh");
  });

  it("falls back to the default locale when nothing matches", () => {
    expect(resolveLocale(undefined, ["fr-FR", "ja-JP"])).toBe(DEFAULT_LOCALE);
    expect(resolveLocale(undefined, undefined)).toBe(DEFAULT_LOCALE);
    expect(resolveLocale("", [])).toBe(DEFAULT_LOCALE);
  });
});

describe("message catalog parity", () => {
  const zhKeys = Object.keys(messages.zh) as MessageKey[];
  const enKeys = Object.keys(messages.en) as MessageKey[];

  it("zh and en expose the same set of keys", () => {
    expect(new Set(zhKeys)).toEqual(new Set(enKeys));
  });

  it("zh is the default locale", () => {
    expect(DEFAULT_LOCALE).toBe("zh");
    expect(zhKeys.length).toBeGreaterThan(0);
  });

  it("every zh key is present in en and vice versa", () => {
    for (const key of zhKeys) {
      expect(enKeys).toContain(key);
    }
    for (const key of enKeys) {
      expect(zhKeys).toContain(key);
    }
  });
});
