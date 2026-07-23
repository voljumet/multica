/**
 * App-level lightbox provider for tap-to-zoom image viewing.
 *
 * Single instance mounted at the root layout. `useLightbox().open(uri)`
 * displays the image fullscreen with pinch-to-zoom, double-tap, and
 * swipe-down-to-dismiss — all handled by `react-native-image-viewing`.
 *
 * Accepts either a plain URI string or an ImageSource with optional
 * `headers` so auth-gated `/api/attachments/<id>/download` URLs can load
 * with a Bearer token (token-mode clients; same path as MarkdownImage).
 *
 * V2.1 only opens single images. A future iteration could collect every
 * `![]()` URL while rendering a comment and pass the array through so
 * a left/right swipe walks the gallery.
 */
import { createContext, use, useState, type ReactNode } from "react";
import ImageView from "react-native-image-viewing";

/** Plain URI or authenticated source (Bearer on auth-gated download URLs). */
export type LightboxSource =
  | string
  | { uri: string; headers?: Record<string, string> };

type LightboxImage = { uri: string; headers?: Record<string, string> };

interface LightboxApi {
  open: (source: LightboxSource) => void;
}

const LightboxContext = createContext<LightboxApi>({
  open: () => {
    // No-op fallback when used outside provider — markdown rendering
    // shouldn't crash if a screen forgets to mount the provider.
  },
});

export function useLightbox(): LightboxApi {
  return use(LightboxContext);
}

function toImageSource(source: LightboxSource): LightboxImage {
  if (typeof source === "string") return { uri: source };
  return source.headers
    ? { uri: source.uri, headers: source.headers }
    : { uri: source.uri };
}

export function LightboxProvider({ children }: { children: ReactNode }) {
  const [images, setImages] = useState<LightboxImage[]>([]);
  const open = (source: LightboxSource) => setImages([toImageSource(source)]);
  const close = () => setImages([]);
  return (
    <LightboxContext.Provider value={{ open }}>
      {children}
      <ImageView
        images={images}
        imageIndex={0}
        visible={images.length > 0}
        onRequestClose={close}
      />
    </LightboxContext.Provider>
  );
}
