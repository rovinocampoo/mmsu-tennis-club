import { NextRequest, NextResponse } from "next/server";
import { isBackendRouteAllowed } from "@/lib/backend-api-paths.mjs";
import { trustedProxyIdentity } from "@/lib/trusted-proxy-identity.mjs";

const apiBase = (process.env.API_BASE_URL || "http://localhost:8080").replace(/\/$/, "");
const proxySharedSecret = process.env.API_PROXY_SHARED_SECRET || "";

async function proxy(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path } = await context.params;
  if (!isBackendRouteAllowed(request.method, path)) return NextResponse.json({ error: "not found" }, { status: 404 });
  if (request.method === "POST" && path[0] === "booking-status" && request.nextUrl.search) {
    return NextResponse.json({ error: "status tokens must be submitted in the request body" }, { status: 400, headers: { "cache-control": "no-store" } });
  }
  const isPublicBookingRequest = request.method === "POST" && ["booking-requests", "booking-status"].includes(path[0]);
  const proxyIdentity = isPublicBookingRequest
    ? trustedProxyIdentity(request.headers, proxySharedSecret, process.env.NODE_ENV === "production")
    : null;
  if (proxyIdentity && "error" in proxyIdentity) {
    return NextResponse.json({ error: "booking service proxy configuration is unavailable" }, { status: 503, headers: { "cache-control": "no-store" } });
  }
  const headers = new Headers();
  const contentType = request.headers.get("content-type");
  const authorization = request.headers.get("authorization");
  if (contentType) headers.set("content-type", contentType);
  if (authorization) headers.set("authorization", authorization);
  if (proxyIdentity) {
    headers.set("x-mmsu-proxy-secret", proxyIdentity.sharedSecret);
    headers.set("x-mmsu-client-ip", proxyIdentity.clientIP);
  }
  try {
    const upstream = await fetch(`${apiBase}/api/${path.map(encodeURIComponent).join("/")}${request.nextUrl.search}`, {
      method: request.method,
      headers,
      body: request.method === "GET" ? undefined : await request.arrayBuffer(),
      cache: "no-store",
      signal: request.signal,
    });
    const responseHeaders = new Headers({ "cache-control": "no-store" });
    const upstreamType = upstream.headers.get("content-type");
    if (upstreamType) responseHeaders.set("content-type", upstreamType);
    return new NextResponse(upstream.body, { status: upstream.status, headers: responseHeaders });
  } catch {
    return NextResponse.json({ error: "booking service is temporarily unavailable" }, { status: 503, headers: { "cache-control": "no-store" } });
  }
}

export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
