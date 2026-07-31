/**
 * Mobile subscriber mutations. Mirrors the optimistic-update + invalidate
 * pattern of packages/core/issues/mutations.ts useToggleIssueSubscription —
 * written here in mobile-owned code per Sharing Principles.
 *
 * Optimistic: flips the local subscriber list immediately (subscribe adds,
 * unsubscribe removes). Post-state is locally predictable, user stays on the
 * same screen, failure is rare, rollback is trivial — qualifies for optimism.
 * Settle invalidate reconciles with the server.
 */
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { IssueSubscriber } from "@multica/core/types/subscriber";
import { api } from "@/data/api";
import { subscriberKeys } from "@/data/queries/subscribers";

export function useToggleSubscription(issueId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      subscribed,
    }: {
      userId: string;
      userType: "member";
      subscribed: boolean;
    }) =>
      subscribed
        ? api.unsubscribeFromIssue(issueId)
        : api.subscribeToIssue(issueId),

    onMutate: async ({ userId, userType, subscribed }) => {
      await qc.cancelQueries({ queryKey: subscriberKeys.all(issueId) });
      const prev = qc.getQueryData<IssueSubscriber[]>(
        subscriberKeys.all(issueId),
      );

      qc.setQueryData<IssueSubscriber[]>(
        subscriberKeys.all(issueId),
        (old) => {
          const list = old ?? [];
          if (subscribed) {
            // Unsubscribing: remove
            return list.filter(
              (s) => !(s.user_id === userId && s.user_type === userType),
            );
          } else {
            // Subscribing: add
            const newSub: IssueSubscriber = {
              issue_id: issueId,
              user_id: userId,
              user_type: userType,
              reason: "manual",
              created_at: new Date().toISOString(),
            };
            return [...list, newSub];
          }
        },
      );

      return { prev };
    },

    onError: (_err, _vars, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(subscriberKeys.all(issueId), ctx.prev);
      }
    },

    onSettled: () => {
      qc.invalidateQueries({ queryKey: subscriberKeys.all(issueId) });
    },
  });
}
