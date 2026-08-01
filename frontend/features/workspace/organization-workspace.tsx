"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { addMember, createOrganization, listMembers, listOrganizations, type OrganizationRole } from "@/lib/api/client";
import { friendlyError, PanelMessage } from "@/features/shared/feedback";
import { RepositoryPanel } from "./repository-panel";

export function OrganizationWorkspace() {
  const queryClient = useQueryClient();
  const [selectedID, setSelectedID] = useState("");
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: () => listOrganizations({ limit: 100 }) });
  const createMutation = useMutation({
    mutationFn: ({ name, description }: { name: string; description: string }) => createOrganization(name, description),
    onSuccess: async (organization) => {
      setSelectedID(organization.id);
      await queryClient.invalidateQueries({ queryKey: ["organizations"] });
    },
  });
  const selected = organizations.data?.items.find((organization) => organization.id === selectedID) ?? organizations.data?.items[0];

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    createMutation.mutate({ name: String(data.get("name") ?? ""), description: String(data.get("description") ?? "") }, { onSuccess: () => form.reset() });
  }

  return (
    <section className="mt-8 scroll-mt-6 rounded-3xl border border-slate-200 bg-white p-5 shadow-sm sm:p-7" id="organizations">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">Organization ownership</p>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">Teams and members</h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">Create a globally unique organization namespace, then add a member under the approved role matrix.</p>
        </div>
      </div>

      <div className="mt-6 grid gap-6 lg:grid-cols-[0.82fr_1.18fr]">
        <div className="space-y-5">
          <form className="space-y-3 rounded-2xl bg-slate-50 p-4" onSubmit={submit}>
            <h3 className="font-semibold text-slate-900">Create organization</h3>
            <div>
              <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor="organization-name">Namespace</label>
              <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="organization-name" name="name" pattern="[A-Za-z0-9]+([._-][A-Za-z0-9]+)*" maxLength={64} required />
            </div>
            <div>
              <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor="organization-description">Description</label>
              <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="organization-description" name="description" maxLength={1024} />
            </div>
            {createMutation.isError ? <p className="text-sm text-rose-700" role="alert">{friendlyError(createMutation.error)}</p> : null}
            <button className="w-full rounded-lg bg-sky-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-sky-700 focus:outline-none focus:ring-4 focus:ring-sky-200 disabled:opacity-60" disabled={createMutation.isPending} type="submit">{createMutation.isPending ? "Creating…" : "Create organization"}</button>
          </form>

          <div>
            <h3 className="text-sm font-semibold text-slate-900">Your organizations</h3>
            <div className="mt-3 space-y-2">
              {organizations.isPending ? <PanelMessage title="Loading organizations" detail="Reading your current memberships." /> : null}
              {organizations.isError ? <PanelMessage title="Organizations unavailable" detail={friendlyError(organizations.error)} tone="error" /> : null}
              {organizations.data?.items.length === 0 ? <PanelMessage title="No organizations yet" detail="Create one to start the organization journey." /> : null}
              {organizations.data?.items.map((organization) => (
                <button className={selected?.id === organization.id ? "w-full rounded-xl border border-sky-300 bg-sky-50 px-4 py-3 text-left focus:outline-none focus:ring-4 focus:ring-sky-100" : "w-full rounded-xl border border-slate-200 px-4 py-3 text-left hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100"} key={organization.id} onClick={() => setSelectedID(organization.id)} type="button">
                  <span className="block font-mono text-sm font-semibold text-slate-950">{organization.namespace}</span>
                  <span className="mt-1 block truncate text-xs text-slate-500">{organization.description || "No description"}</span>
                </button>
              ))}
              {organizations.data?.meta.next_cursor ? <p className="text-xs text-slate-500">Showing the first 100 organizations.</p> : null}
            </div>
          </div>
        </div>

        <div className="min-w-0">
          {selected ? <OrganizationDetail organizationID={selected.id} namespace={selected.namespace} /> : <PanelMessage title="Select an organization" detail="Organization members and repositories will appear here." />}
        </div>
      </div>
    </section>
  );
}

function OrganizationDetail({ organizationID, namespace }: Readonly<{ organizationID: string; namespace: string }>) {
  const queryClient = useQueryClient();
  const membersKey = ["organizations", organizationID, "members"] as const;
  const members = useQuery({ queryKey: membersKey, queryFn: () => listMembers(organizationID, { limit: 100 }) });
  const addMutation = useMutation({
    mutationFn: ({ userID, role }: { userID: string; role: OrganizationRole }) => addMember(organizationID, userID, role),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: membersKey }); },
  });

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    addMutation.mutate({ userID: String(data.get("user_id") ?? ""), role: String(data.get("role") ?? "READER") as OrganizationRole }, { onSuccess: () => form.reset() });
  }

  return (
    <div className="space-y-6">
      <div className="rounded-2xl border border-slate-200 p-5">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-mono text-xs text-sky-700">hubcr.io/{namespace}</p>
            <h3 className="mt-1 text-lg font-semibold text-slate-950">Members</h3>
          </div>
          <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-600">Role policy</span>
        </div>
        <form className="mt-4 grid gap-3 sm:grid-cols-[1fr_auto_auto]" onSubmit={submit}>
          <div>
            <label className="sr-only" htmlFor={`${organizationID}-member-id`}>User ID</label>
            <input className="w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${organizationID}-member-id`} name="user_id" placeholder="User UUID" pattern="[0-9a-f-]{36}" required />
          </div>
          <label className="sr-only" htmlFor={`${organizationID}-member-role`}>Role</label>
          <select className="rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" defaultValue="READER" id={`${organizationID}-member-role`} name="role">
            {(["OWNER", "ADMIN", "WRITER", "READER"] as const).map((role) => <option key={role}>{role}</option>)}
          </select>
          <button className="rounded-lg bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200 disabled:opacity-60" disabled={addMutation.isPending} type="submit">{addMutation.isPending ? "Adding…" : "Add member"}</button>
        </form>
        {addMutation.isError ? <p className="mt-3 text-sm text-rose-700" role="alert">{friendlyError(addMutation.error)}</p> : null}
        <div className="mt-4 space-y-2">
          {members.isPending ? <PanelMessage title="Loading members" detail="Reading the authorized membership list." /> : null}
          {members.isError ? <PanelMessage title="Members unavailable" detail={friendlyError(members.error)} tone="error" /> : null}
          {members.data?.items.length === 0 ? <PanelMessage title="No members returned" detail="The organization currently has no visible membership data." /> : null}
          {members.data?.items.map((member) => (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-slate-50 px-3 py-2.5" key={member.user_id}>
              <span className="truncate font-mono text-xs text-slate-700">{member.user_id}</span>
              <span className="rounded-full bg-white px-2.5 py-1 text-xs font-semibold text-slate-700 shadow-sm">{member.role}</span>
            </div>
          ))}
        </div>
      </div>
      <RepositoryPanel namespace={namespace} title="Organization repositories" />
    </div>
  );
}
