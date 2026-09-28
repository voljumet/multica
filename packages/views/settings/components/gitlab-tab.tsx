"use client";

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowRight, Tag } from "lucide-react";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace } from "@multica/core/paths";
import { memberListOptions, workspaceKeys } from "@multica/core/workspace/queries";
import { deriveGitLabSettings, DEFAULT_GITLAB_ISSUE_SYNC_LABEL } from "@multica/core/gitlab";
import { api } from "@multica/core/api";
import type { Workspace } from "@multica/core/types";
import { useT } from "../../i18n";
import { SettingsSaveState } from "./settings-layout";
import { useAutoSave } from "./use-auto-save";

export function GitLabTab() {
  const { t } = useT("settings");
  const workspace = useCurrentWorkspace();
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const user = useAuthStore((s) => s.user);
  const [savingIssueSync, setSavingIssueSync] = useState(false);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage = currentMember?.role === "owner" || currentMember?.role === "admin";

  const flags = deriveGitLabSettings(workspace);
  const [issueSyncLabelDraft, setIssueSyncLabelDraft] = useState(flags.issueSyncLabel);

  useEffect(() => {
    setIssueSyncLabelDraft(flags.issueSyncLabel);
    // Cache updates after auto-save replace the Workspace object. Keying on
    // identity prevents that response from wiping a newer local keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally keyed on workspace identity
  }, [workspace?.id]);

  async function persistWorkspaceSettings(patch: Record<string, unknown>) {
    if (!workspace) return;
    const cached = qc.getQueryData<Workspace[]>(workspaceKeys.list())?.find(
      (ws) => ws.id === workspace.id,
    );
    const base = cached ?? workspace;
    const merged = { ...((base.settings as Record<string, unknown>) ?? {}), ...patch };
    const updated = await api.updateWorkspace(workspace.id, { settings: merged });
    qc.setQueryData(workspaceKeys.list(), (old: Workspace[] | undefined) =>
      old?.map((ws) => (ws.id === updated.id ? updated : ws)),
    );
  }

  const labelAutoSave = useAutoSave({
    value: issueSyncLabelDraft,
    savedValue: flags.issueSyncLabel,
    enabled: canManage && !!workspace,
    onSave: async (value) => {
      const next = value.trim() || DEFAULT_GITLAB_ISSUE_SYNC_LABEL;
      await persistWorkspaceSettings({ gitlab_issue_sync_label: next });
      if (next !== value) {
        setIssueSyncLabelDraft(next);
      }
    },
    onError: (e) => {
      toast.error(e instanceof Error ? e.message : t(($) => $.gitlab.toast_failed));
    },
    isEqual: (a, b) =>
      a.trim() === b.trim() || (a.trim() === "" && b === DEFAULT_GITLAB_ISSUE_SYNC_LABEL),
  });

  async function toggleIssueSync(next: boolean) {
    if (!workspace || savingIssueSync) return;
    setSavingIssueSync(true);
    try {
      await persistWorkspaceSettings({ gitlab_issue_sync_enabled: next });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.gitlab.toast_failed));
    } finally {
      setSavingIssueSync(false);
    }
  }

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">{t(($) => $.gitlab.section_features)}</h2>
          <SettingsSaveState
            status={labelAutoSave.status}
            savingLabel={t(($) => $.auto_save.saving)}
            savedLabel={t(($) => $.auto_save.saved)}
            errorLabel={t(($) => $.auto_save.failed)}
          />
        </div>
        <Card>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {t(($) => $.gitlab.connection_hint)}
            </p>
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
                  <Tag className="h-4 w-4" />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="gitlab-issue-sync" className="text-sm font-medium">
                    {t(($) => $.gitlab.feature_issue_sync_label)}
                  </Label>
                  <p className="text-sm text-muted-foreground">
                    {t(($) => $.gitlab.feature_issue_sync_description)}
                  </p>
                </div>
              </div>
              <Switch
                id="gitlab-issue-sync"
                checked={flags.issueSync}
                disabled={!canManage || savingIssueSync}
                onCheckedChange={toggleIssueSync}
              />
            </div>
            <div className="flex items-start justify-between gap-4 border-t pt-4">
              <div className="space-y-1">
                <Label htmlFor="gitlab-issue-sync-label" className="text-sm font-medium">
                  {t(($) => $.gitlab.issue_sync_label_label)}
                </Label>
                <p className="text-sm text-muted-foreground">
                  {t(($) => $.gitlab.issue_sync_label_description)}
                </p>
              </div>
              <Input
                id="gitlab-issue-sync-label"
                value={issueSyncLabelDraft}
                onChange={(e) => setIssueSyncLabelDraft(e.target.value)}
                onBlur={labelAutoSave.flush}
                disabled={!canManage}
                placeholder={DEFAULT_GITLAB_ISSUE_SYNC_LABEL}
                spellCheck={false}
                autoComplete="off"
                className="max-w-[12rem] font-mono text-xs"
              />
            </div>
          </CardContent>
        </Card>
      </section>

      <section className="space-y-3">
        <h2 className="text-sm font-semibold">{t(($) => $.gitlab.section_status)}</h2>
        <Card>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">{t(($) => $.gitlab.status_intro)}</p>
            <ul className="divide-y rounded-md border">
              {[
                {
                  key: "close",
                  event: t(($) => $.gitlab.status_close),
                  result: t(($) => $.gitlab.status_close_result),
                },
                {
                  key: "reopen",
                  event: t(($) => $.gitlab.status_reopen),
                  result: t(($) => $.gitlab.status_reopen_result),
                },
                {
                  key: "label_remove",
                  event: t(($) => $.gitlab.status_label_remove),
                  result: t(($) => $.gitlab.status_label_remove_result),
                },
                {
                  key: "label_restore",
                  event: t(($) => $.gitlab.status_label_restore),
                  result: t(($) => $.gitlab.status_label_restore_result),
                },
              ].map((row) => (
                <li
                  key={row.key}
                  className="flex flex-col gap-1 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4"
                >
                  <span className="text-sm text-foreground">{row.event}</span>
                  <span className="flex items-center gap-1.5 text-sm text-muted-foreground sm:shrink-0">
                    <ArrowRight className="hidden h-3.5 w-3.5 sm:block" aria-hidden />
                    <span className="font-medium text-foreground">{row.result}</span>
                  </span>
                </li>
              ))}
            </ul>
            <p className="text-sm text-muted-foreground">{t(($) => $.gitlab.status_note)}</p>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
