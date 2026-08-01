import { ControlPlaneWorkspace } from "@/features/workspace/control-plane-workspace";

export default function Home() {
  return (
    <main className="mx-auto min-h-screen w-full max-w-7xl px-5 py-6 sm:px-8 sm:py-8 lg:px-10">
      <header className="mb-8 flex items-center justify-between border-b border-slate-200 pb-5">
        <div className="flex items-center gap-3">
          <span className="grid size-9 place-items-center rounded-xl bg-slate-950 font-mono text-sm font-bold text-white">H</span>
          <div>
            <span className="block text-lg font-semibold tracking-tight text-slate-950">HubCR</span>
            <span className="block text-xs text-slate-500">Container Registry</span>
          </div>
        </div>
        <span className="rounded-full border border-sky-200 bg-sky-50 px-3 py-1 text-xs font-semibold text-sky-700">Registry MVP · M1</span>
      </header>
      <ControlPlaneWorkspace />
      <footer className="mt-10 border-t border-slate-200 py-6 text-xs leading-5 text-slate-500">
        HubCR business control plane · OCI content transfer is provided by CNCF Distribution.
      </footer>
    </main>
  );
}
