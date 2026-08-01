import { AuthenticatedShell } from "@/features/auth/authenticated-shell";

export default function WorkspaceLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <AuthenticatedShell>{children}</AuthenticatedShell>;
}
