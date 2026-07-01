import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { workspaceKeys } from "../workspace/queries";

export interface CopyAgentParams {
  agentId: string;
  targetWorkspaceSlug: string;
  name?: string;
  targetRuntimeId?: string;
}

// useCopyAgent deploys an agent to another workspace by duplicating its
// configuration (name, instructions, skills). Secrets are never copied.
// If the source runtime has visibility='shared', the copy binds to it
// automatically; otherwise the caller must supply targetRuntimeId.
export function useCopyAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, targetWorkspaceSlug, name, targetRuntimeId }: CopyAgentParams) =>
      api.copyAgent(agentId, {
        target_workspace_slug: targetWorkspaceSlug,
        name,
        target_runtime_id: targetRuntimeId,
      }),
    onSuccess: (agent) => {
      // Invalidate agent list for the target workspace so it shows up immediately.
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(agent.workspace_id) });
    },
  });
}
