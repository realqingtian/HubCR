import Link from "next/link";
import { DEFAULT_LOCALE, messages } from "@/lib/i18n/messages";

export default function NotFound() {
  const m = messages[DEFAULT_LOCALE];
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
      <p className="text-xs font-semibold uppercase tracking-[0.18em] text-rose-700">{m["route.invalidRoute"]}</p>
      <h1 className="mt-2 text-2xl font-semibold text-slate-950">{m["route.namespaceNotFound"]}</h1>
      <p className="mt-3 text-sm leading-6 text-slate-600">{m["route.namespaceNotFoundDetail"]}</p>
      <Link className="mt-5 inline-flex rounded-lg bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200" href="/">{m["route.backOverview"]}</Link>
    </section>
  );
}
