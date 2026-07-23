import { describe, expect, it } from "vitest";
import type { Attachment } from "@multica/core/types";
import {
  findAttachmentForUri,
  isAttachmentDownloadURL,
  isSignedMediaURL,
  pickInlineMediaURL,
  resolveInlineMediaUrl,
} from "./resolve-inline-media-url";

const ID = "11111111-2222-3333-4444-555555555555";
const BASE = "https://api.example.test";

function makeAtt(overrides: Partial<Attachment> = {}): Attachment {
  return {
    id: ID,
    workspace_id: "ws-1",
    issue_id: null,
    comment_id: null,
    chat_session_id: null,
    chat_message_id: null,
    uploader_type: "member",
    uploader_id: "u-1",
    filename: "shot.png",
    url: "https://cdn.example.test/uploads/ws/shot.png",
    download_url: `/api/attachments/${ID}/download`,
    markdown_url: `https://api.example.test/api/attachments/${ID}/download`,
    content_type: "image/png",
    size_bytes: 1024,
    created_at: "2026-05-13T00:00:00Z",
    ...overrides,
  };
}

describe("isAttachmentDownloadURL / isSignedMediaURL", () => {
  it("recognizes absolute and relative download endpoints", () => {
    expect(isAttachmentDownloadURL(`/api/attachments/${ID}/download`)).toBe(
      true,
    );
    expect(
      isAttachmentDownloadURL(
        `https://api.example.test/api/attachments/${ID}/download`,
      ),
    ).toBe(true);
    expect(isAttachmentDownloadURL("https://cdn.example.test/shot.png")).toBe(
      false,
    );
  });

  it("recognizes CloudFront / S3 signed query strings", () => {
    expect(
      isSignedMediaURL(
        "https://cdn.example.test/shot.png?Signature=s&Key-Pair-Id=k",
      ),
    ).toBe(true);
    expect(
      isSignedMediaURL(
        "https://bucket.s3.amazonaws.com/shot.png?X-Amz-Signature=s&X-Amz-Expires=900",
      ),
    ).toBe(true);
    expect(isSignedMediaURL("https://cdn.example.test/shot.png")).toBe(false);
    expect(isSignedMediaURL(`/api/attachments/${ID}/download`)).toBe(false);
  });
});

describe("findAttachmentForUri", () => {
  const att = makeAtt();

  it("matches the durable markdown_url (the form web/CLI persist)", () => {
    expect(findAttachmentForUri(att.markdown_url, [att])).toBe(att);
  });

  it("matches a site-relative download path by id", () => {
    expect(
      findAttachmentForUri(`/api/attachments/${ID}/download`, [att]),
    ).toBe(att);
  });

  it("matches mc://file/<id>", () => {
    expect(findAttachmentForUri(`mc://file/${ID}`, [att])).toBe(att);
  });

  it("matches raw storage url (legacy markdown)", () => {
    expect(findAttachmentForUri(att.url, [att])).toBe(att);
  });

  it("returns undefined for external images", () => {
    expect(
      findAttachmentForUri("https://external.example/photo.png", [att]),
    ).toBeUndefined();
  });
});

describe("pickInlineMediaURL", () => {
  it("prefers a signed absolute download_url", () => {
    const signed =
      "https://cdn.example.test/shot.png?Signature=fresh&Key-Pair-Id=K";
    const att = makeAtt({ download_url: signed });
    expect(pickInlineMediaURL(att, "fallback")).toBe(signed);
  });

  it("falls through to markdown_url when download_url is the API path", () => {
    const att = makeAtt({
      download_url: `/api/attachments/${ID}/download`,
    });
    expect(pickInlineMediaURL(att, "fallback")).toBe(att.markdown_url);
  });

  it("returns the fallback when no record is present", () => {
    expect(pickInlineMediaURL(undefined, "https://external/x.png")).toBe(
      "https://external/x.png",
    );
  });
});

describe("resolveInlineMediaUrl", () => {
  it("resolves markdown_url content to an absolute auth-gated download URL and flags needsAuth", () => {
    const att = makeAtt({
      download_url: `/api/attachments/${ID}/download`,
    });
    const resolved = resolveInlineMediaUrl(att.markdown_url, [att], BASE);
    expect(resolved.uri).toBe(
      `https://api.example.test/api/attachments/${ID}/download`,
    );
    expect(resolved.attachmentId).toBe(ID);
    expect(resolved.needsAuth).toBe(true);
  });

  it("uses a signed download_url without needing auth headers", () => {
    const signed =
      "https://cdn.example.test/shot.png?Signature=fresh&Key-Pair-Id=K";
    const att = makeAtt({ download_url: signed });
    const resolved = resolveInlineMediaUrl(att.markdown_url, [att], BASE);
    expect(resolved.uri).toBe(signed);
    expect(resolved.needsAuth).toBe(false);
  });

  it("absolutizes a site-relative download path from the record", () => {
    const att = makeAtt({
      download_url: `/api/attachments/${ID}/download`,
      markdown_url: "",
    });
    // Input is the relative path (legacy markdown). Match by id, pick
    // download_url (also relative), absolutize against base.
    const resolved = resolveInlineMediaUrl(
      `/api/attachments/${ID}/download`,
      [att],
      BASE,
    );
    expect(resolved.uri).toBe(
      `https://api.example.test/api/attachments/${ID}/download`,
    );
    expect(resolved.needsAuth).toBe(true);
  });

  it("passes external https URIs through unchanged", () => {
    const external = "https://images.example/photo.png";
    const resolved = resolveInlineMediaUrl(external, [], BASE);
    expect(resolved.uri).toBe(external);
    expect(resolved.attachmentId).toBeUndefined();
    expect(resolved.needsAuth).toBe(false);
  });

  it("resolves mc://file/<id> via the attachments list", () => {
    const signed =
      "https://cdn.example.test/shot.png?Signature=fresh&Key-Pair-Id=K";
    const att = makeAtt({ download_url: signed });
    const resolved = resolveInlineMediaUrl(`mc://file/${ID}`, [att], BASE);
    expect(resolved.uri).toBe(signed);
    expect(resolved.attachmentId).toBe(ID);
    expect(resolved.needsAuth).toBe(false);
  });
});
