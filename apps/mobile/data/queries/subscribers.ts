import { queryOptions } from "@tanstack/react-query";
import { api } from "@/data/api";

/**
 * Subscriber cache key factory.
 *
 * Two-segment shape: `["subscribers", issueId]`. No workspace segment because
 * issue IDs are globally unique and the endpoint is scoped by issueId alone.
 * Mirrors the mental model of other per-issue caches (timeline, attachments).
 */
export const subscriberKeys = {
  all: (issueId: string) => ["subscribers", issueId] as const,
};

export function subscribersOptions(issueId: string) {
  return queryOptions({
    queryKey: subscriberKeys.all(issueId),
    queryFn: ({ signal }) => api.listIssueSubscribers(issueId, { signal }),
    enabled: !!issueId,
  });
}
