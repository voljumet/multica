/**
 * Block-level image with real aspect ratio + tap-to-lightbox.
 *
 *   - Aspect ratio detection uses RN's `Image.getSize` / `getSizeWithHeaders`
 *     (cross-platform, network-friendly). While dimensions resolve we lay
 *     out at 16:9 as a placeholder — same width-100% so the surrounding
 *     flow is stable and only the height shifts once the real ratio lands.
 *   - Rendering uses `expo-image` for on-disk caching + smooth fade-in
 *     transition.
 *   - Tap dispatches into the global LightboxProvider for fullscreen
 *     viewing with pinch-zoom + swipe-down-to-dismiss.
 *
 * URI resolution (MUL-3130 / MUL-3254 parity with web/desktop):
 *
 *   Markdown stores durable references (`markdown_url` or the stable
 *   `/api/attachments/<id>/download` path), never short-lived signed CDN
 *   URLs. We look the URI up in the surrounding `attachments` list, prefer
 *   a freshly-signed absolute `download_url` when the list has one, and
 *   re-sign via authenticated `getAttachment` when the picked URL is still
 *   the auth-gated API endpoint. Token-mode native loaders (RN / iOS)
 *   cannot attach Authorization to bare resource fetches, so without this
 *   hop the image is a blank frame and the lightbox spins forever.
 *
 *   When re-sign still yields an API path (non-CloudFront deployments that
 *   proxy the bytes), we pass the Bearer token as request headers on both
 *   expo-image and the lightbox so the authenticated download endpoint
 *   can stream the body.
 *
 * Cancellation: a content re-render that swaps the URI must not let the
 * previous getSize callback overwrite state — guard with a `cancelled`
 * flag in the cleanup path.
 */
import { useEffect, useMemo, useState } from "react";
import { Image as RNImage, Pressable, View } from "react-native";
import { Image as ExpoImage } from "expo-image";
import { useQuery } from "@tanstack/react-query";
import type { Attachment } from "@multica/core/types";
import { api } from "@/data/api";
import {
  isSignedMediaURL,
  resolveInlineMediaUrl,
} from "@/lib/resolve-inline-media-url";
import { useLightbox } from "./lightbox-provider";

const API_URL = process.env.EXPO_PUBLIC_API_URL ?? "";
const RESIGN_STALE_MS = 20 * 60 * 1000;

interface Props {
  uri: string;
  alt?: string;
  attachments?: Attachment[];
}

export function MarkdownImage({ uri, attachments }: Props) {
  const { open } = useLightbox();
  const [aspect, setAspect] = useState<number | null>(null);

  const base = useMemo(
    () => resolveInlineMediaUrl(uri, attachments, API_URL),
    [uri, attachments],
  );

  // Token-mode re-sign: when the resolved URI is still the auth-gated
  // download endpoint, fetch fresh metadata so CloudFront / S3 deployments
  // can hand us a signed absolute URL that loads without headers.
  const { data: fresh } = useQuery({
    queryKey: ["attachment-inline-resign", base.attachmentId],
    queryFn: ({ signal }) =>
      api.getAttachment(base.attachmentId as string, { signal }),
    enabled: !!base.attachmentId && base.needsAuth,
    staleTime: RESIGN_STALE_MS,
    gcTime: RESIGN_STALE_MS,
  });

  const media = useMemo(() => {
    const dl = fresh?.download_url ?? "";
    if (isSignedMediaURL(dl)) {
      return { uri: dl, headers: undefined as Record<string, string> | undefined };
    }
    // Still on the API download shape (or external). Attach Bearer when the
    // endpoint needs auth so expo-image / lightbox can load proxy mode.
    const headers =
      base.needsAuth && !isSignedMediaURL(base.uri)
        ? api.getAuthHeaders()
        : undefined;
    return {
      uri: base.uri,
      headers: headers && Object.keys(headers).length > 0 ? headers : undefined,
    };
  }, [base.needsAuth, base.uri, fresh?.download_url]);

  useEffect(() => {
    let cancelled = false;
    const onSuccess = (w: number, h: number) => {
      if (cancelled || !w || !h) return;
      setAspect(w / h);
    };
    const onFail = () => {
      // Network failure / decode failure / 404 / unknown URI scheme —
      // keep the 16:9 fallback so the slot still shows the muted
      // background instead of collapsing.
      if (!cancelled) setAspect(16 / 9);
    };

    if (media.headers) {
      RNImage.getSizeWithHeaders(media.uri, media.headers, onSuccess, onFail);
    } else {
      RNImage.getSize(media.uri, onSuccess, onFail);
    }
    return () => {
      cancelled = true;
    };
  }, [media.headers, media.uri]);

  return (
    <Pressable
      onPress={() =>
        open(
          media.headers
            ? { uri: media.uri, headers: media.headers }
            : media.uri,
        )
      }
    >
      <View className="rounded-lg overflow-hidden bg-muted">
        <ExpoImage
          source={
            media.headers
              ? { uri: media.uri, headers: media.headers }
              : { uri: media.uri }
          }
          style={{ width: "100%", aspectRatio: aspect ?? 16 / 9 }}
          contentFit="contain"
          transition={150}
        />
      </View>
    </Pressable>
  );
}
