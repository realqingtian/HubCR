import { APIError } from "@/lib/api/client";
import { useT } from "@/lib/i18n";

// useFriendlyError returns a function that translates an error into a localized message
// using the active locale. Components should call this hook and then invoke the returned
// function where a human string is needed.
export function useFriendlyError(): (error: unknown) => string {
  const t = useT();
  return (error: unknown): string => {
    if (error instanceof APIError) {
      switch (error.code) {
        case "authentication_failed":
          return t("shared.friendlyError.authentication_failed");
        case "forbidden":
          return t("shared.friendlyError.forbidden");
        case "conflict":
          return t("shared.friendlyError.conflict");
        case "validation_failed":
          // Backend field messages are English and intentionally left untranslated
          // (see i18n design decision 2026-08-09).
          return error.fields[0]?.message ?? t("shared.friendlyError.validation_failed");
        default:
          return error.requestID
            ? `${error.message} (request ${error.requestID})`
            : error.message;
      }
    }
    return t("shared.friendlyError.default");
  };
}

// friendlyError remains for callers that cannot use a hook; it renders the default locale.
// Prefer useFriendlyError() inside components.
export function friendlyError(error: unknown): string {
  if (error instanceof APIError) {
    switch (error.code) {
      case "authentication_failed":
        return "Your session is no longer valid. Sign in again.";
      case "forbidden":
        return "Your account does not have permission for this action.";
      case "conflict":
        return "That name or membership is already in use.";
      case "validation_failed":
        return error.fields[0]?.message ?? "Check the submitted values and try again.";
      default:
        return error.requestID
          ? `${error.message} (request ${error.requestID})`
          : error.message;
    }
  }
  return "The control plane is unavailable. Check the API connection and try again.";
}

export function PanelMessage({
  title,
  detail,
  tone = "neutral",
}: Readonly<{
  title: string;
  detail: string;
  tone?: "neutral" | "error";
}>) {
  const colors =
    tone === "error"
      ? "border-rose-200 bg-rose-50 text-rose-950"
      : "border-slate-200 bg-slate-50 text-slate-700";
  return (
    <div className={`rounded-xl border px-4 py-3 ${colors}`} role={tone === "error" ? "alert" : "status"}>
      <p className="text-sm font-semibold">{title}</p>
      <p className="mt-1 text-sm leading-6 opacity-80">{detail}</p>
    </div>
  );
}
