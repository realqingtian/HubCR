import type { QueryClient } from "@tanstack/react-query";
import type { User } from "@/lib/api/client";

export const currentUserQueryKey = ["auth", "me"] as const;

export function installAuthenticatedUser(queryClient: QueryClient, user: User) {
  queryClient.removeQueries({
    predicate: (query) => !isCurrentUserQuery(query.queryKey),
  });
  queryClient.getMutationCache().clear();
  queryClient.setQueryData(currentUserQueryKey, user);
}

function isCurrentUserQuery(queryKey: readonly unknown[]) {
  return queryKey.length === currentUserQueryKey.length &&
    queryKey.every((part, index) => part === currentUserQueryKey[index]);
}
