import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { currentUserQueryKey, installAuthenticatedUser } from "./query-cache";

describe("authenticated query cache transitions", () => {
  it("removes the previous principal's private data while preserving the active auth query", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(currentUserQueryKey, { id: "account-a" });
    queryClient.setQueryData(["repositories", "private"], [{ name: "account-a-secret" }]);
    queryClient.getMutationCache().build(queryClient, {
      mutationKey: ["repositories", "update"],
      mutationFn: async () => ({ name: "account-a-secret" }),
    });
    const currentUserObserver = new QueryObserver(queryClient, {
      queryKey: currentUserQueryKey,
    });
    const unsubscribe = currentUserObserver.subscribe(() => undefined);

    const accountB = {
      id: "22222222-2222-4222-8222-222222222222",
      username: "account-b",
      personal_namespace: "account-b",
      created_at: "2026-08-08T12:00:00Z",
    };
    installAuthenticatedUser(queryClient, accountB);

    expect(queryClient.getQueryData(["repositories", "private"])).toBeUndefined();
    expect(queryClient.getQueryData(currentUserQueryKey)).toEqual(accountB);
    expect(queryClient.getQueryCache().getAll()).toHaveLength(1);
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
    expect(currentUserObserver.getCurrentResult().data).toEqual(accountB);
    unsubscribe();
  });
});
