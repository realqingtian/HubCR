import { APIError } from "@/lib/api/client";

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
