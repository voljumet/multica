/**
 * Resolve a markdown image URI to a loadable media URL for React Native.
 *
 * Mirrors the web/desktop Attachment resolver in
 * `packages/views/editor/attachment-download-context.tsx` +
 * `pickInlineMediaURL` in `packages/views/editor/attachment.tsx`:
 *
 *   1. Match the markdown URI to an attachment record by id (stable
 *      `/api/attachments/<id>/download` form) or by any of the URL fields
 *      the server emits (`url` / `download_url` / `markdown_url`).
 *   2. Prefer a short-lived signed `download_url` when one is present —
 *      native image loaders cannot attach Authorization headers, so an
 *      absolute signed CDN / S3 URL is the only shape that loads without
 *      an auth hop.
 *   3. Otherwise fall through to `markdown_url` / raw `url` / the input.
 *   4. Absolutize any server-relative path against the API base.
 *
 * The previous mobile matcher only compared `a.url === uri`. Post-MUL-3130
 * content embeds the durable `markdown_url` (or the stable download path),
 * so that equality almost never hit — images stayed on the auth-gated API
 * endpoint and iOS rendered an empty frame + eternal lightbox spinner.
 */

import type { Attachment } from "@multica/core/types";
import { attachmentIdFromDownloadURL } from "@multica/core/types";
import { resolveAttachmentUrlWithBase } from "./attachment-url";

const SIGNED_QUERY_RE =
  /[?&](Signature|X-Amz-Signature|Key-Pair-Id|Expires|X-Amz-Expires)=/i;

const MC_FILE_RE = /^mc:\/\/file\/([^/?#]+)$/i;

function stripQueryAndFragment(url: string): string {
  return url.split(/[?#]/, 1)[0] ?? "";
}

function matchesAttachmentURL(
  embeddedURL: string,
  attachmentURL?: string,
): boolean {
  if (!embeddedURL || !attachmentURL) return false;
  if (embeddedURL === attachmentURL) return true;
  const embeddedStable = stripQueryAndFragment(embeddedURL);
  const attachmentStable = stripQueryAndFragment(attachmentURL);
  return embeddedStable !== "" && embeddedStable === attachmentStable;
}

/** True when `url` is the stable per-attachment download endpoint. */
export function isAttachmentDownloadURL(url: string): boolean {
  return attachmentIdFromDownloadURL(url) !== undefined;
}

/** True when `url` carries a short-lived signature query (CF / S3 presign). */
export function isSignedMediaURL(url: string): boolean {
  if (!url || !/^https?:\/\//i.test(url)) return false;
  return SIGNED_QUERY_RE.test(url);
}

/**
 * Find the attachment record a markdown image URI refers to, or undefined
 * for external / unknown links.
 */
export function findAttachmentForUri(
  uri: string,
  attachments: Attachment[] | undefined,
): Attachment | undefined {
  if (!uri || !attachments?.length) return undefined;

  const idFromUrl = attachmentIdFromDownloadURL(uri);
  if (idFromUrl) {
    const byId = attachments.find((a) => a.id === idFromUrl);
    if (byId) return byId;
  }

  const mcMatch = uri.match(MC_FILE_RE);
  if (mcMatch?.[1]) {
    const byMc = attachments.find((a) => a.id === mcMatch[1]);
    if (byMc) return byMc;
  }

  return attachments.find(
    (a) =>
      matchesAttachmentURL(uri, a.url) ||
      matchesAttachmentURL(uri, a.download_url) ||
      matchesAttachmentURL(uri, a.markdown_url),
  );
}

/**
 * Pick the URL most likely to load as a native image resource without
 * Authorization. Signed absolute `download_url` wins; otherwise durable
 * markdown / storage / input fallbacks.
 */
export function pickInlineMediaURL(
  record: Attachment | undefined,
  fallback: string,
): string {
  if (!record) return fallback;
  const dl = record.download_url ?? "";
  if (isSignedMediaURL(dl)) return dl;
  if (record.markdown_url) return record.markdown_url;
  if (dl) return dl;
  if (record.url) return record.url;
  return fallback;
}

export interface ResolvedInlineMedia {
  /** Absolutized URL ready for Image / expo-image / lightbox. */
  uri: string;
  /** Attachment id when the URI maps to a Multica attachment, else undefined. */
  attachmentId: string | undefined;
  /**
   * True when `uri` is still the auth-gated download endpoint and needs
   * either a re-signed CDN URL or an Authorization header on the request.
   */
  needsAuth: boolean;
}

/**
 * Resolve a markdown image URI against the surrounding attachments list
 * and an API base URL (for server-relative paths).
 */
export function resolveInlineMediaUrl(
  uri: string,
  attachments: Attachment[] | undefined,
  apiBaseUrl: string,
): ResolvedInlineMedia {
  const record = findAttachmentForUri(uri, attachments);
  const picked = pickInlineMediaURL(record, uri);
  const absolute = resolveAttachmentUrlWithBase(picked, apiBaseUrl) ?? picked;
  const attachmentId =
    record?.id ??
    attachmentIdFromDownloadURL(uri) ??
    attachmentIdFromDownloadURL(absolute);
  return {
    uri: absolute,
    attachmentId,
    needsAuth: isAttachmentDownloadURL(absolute),
  };
}
