/**
 * Description block. Renders markdown via the standalone mobile markdown
 * renderer at apps/mobile/lib/markdown/. Empty / null descriptions show
 * a muted "No description." placeholder rather than collapsing the block,
 * so the layout above the timeline stays stable when the user adds a
 * description later.
 *
 * Attachments are fetched per-issue so markdown can resolve durable image
 * references (`markdown_url`, `/api/attachments/<id>/download`, `mc://file/<id>`)
 * to a loadable media URL (signed CDN download_url, or the API download
 * endpoint with Authorization headers). Without this list, iOS cannot load
 * auth-gated Multica attachments and shows an empty frame / eternal lightbox
 * spinner. TanStack Query dedupes the request across this component and
 * CommentCard (both call `issueAttachmentsOptions(wsId, issueId)`), so only
 * one network roundtrip fires per issue.
 */
import { View } from "react-native";
import { useQuery } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { Markdown } from "@/lib/markdown";
import { issueAttachmentsOptions } from "@/data/queries/issues";
import { useWorkspaceStore } from "@/data/workspace-store";

export function IssueDescription({
  issueId,
  description,
}: {
  issueId: string;
  description: string | null;
}) {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const { data: attachments } = useQuery(
    issueAttachmentsOptions(wsId, issueId),
  );

  if (!description || description.trim().length === 0) {
    return (
      <View className="px-4 pb-4">
        <Text className="text-sm text-muted-foreground italic">
          No description.
        </Text>
      </View>
    );
  }
  return (
    <View className="px-4 pb-4">
      <Markdown content={description} attachments={attachments} />
    </View>
  );
}
