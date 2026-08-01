export default function Loading() {
  return (
    <div className="space-y-4" role="status">
      <p className="text-sm font-semibold text-slate-700">Loading workspace route</p>
      <div className="h-36 animate-pulse rounded-3xl bg-slate-200" />
      <div className="h-24 animate-pulse rounded-2xl bg-slate-100" />
    </div>
  );
}
