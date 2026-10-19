import { uploadSpeed } from "@/lib/uploadGroups";

export type ApiError = { error: string };

export type Captcha = { id: string; image: string };

export class RequestError extends Error {
  captchaRequired?: boolean;
  captcha?: Captcha;
  log?: string;
}

export class UploadCancelled extends Error {
  constructor() {
    super("Upload cancelled");
    this.name = "UploadCancelled";
  }
}

export function uploadWasCancelled(err: unknown) {
  return err instanceof UploadCancelled;
}

let leavingForLogin = false;

export function shouldGoToLogin(status: number, requestURL: string, pagePath: string) {
  if (status !== 401) return false;
  const path = requestURL.split("?")[0];
  if (path === "/api/login" || path === "/api/setup" || path === "/api/login/captcha" || path.startsWith("/api/setup/")) return false;
  if (pagePath === "/login" || pagePath === "/setup") return false;
  return true;
}

export function redirectIfSessionExpired(status: number, requestURL: string) {
  if (typeof window === "undefined") return false;
  if (!shouldGoToLogin(status, requestURL, window.location.pathname)) return false;
  if (!leavingForLogin) {
    leavingForLogin = true;
    window.location.assign("/login");
  }
  return true;
}

async function parse<T>(res: Response, url: string): Promise<T> {
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (redirectIfSessionExpired(res.status, url)) return new Promise<T>(() => {});
    const payload = data as ApiError & { captchaRequired?: boolean; captcha?: Captcha; log?: string };
    const err = new RequestError(payload.error || res.statusText);
    err.captchaRequired = payload.captchaRequired;
    err.captcha = payload.captcha;
    err.log = payload.log;
    throw err;
  }
  return data as T;
}

export const api = {
  get: <T>(url: string) => fetch(url, { credentials: "include", cache: "no-store" }).then((res) => parse<T>(res, url)),
  send: <T>(url: string, method: string, body?: unknown) =>
    fetch(url, {
      method,
      credentials: "include",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    }).then((res) => parse<T>(res, url)),
  post: <T>(url: string, body?: unknown) => api.send<T>(url, "POST", body),
  put: <T>(url: string, body?: unknown) => api.send<T>(url, "PUT", body),
  patch: <T>(url: string, body?: unknown) => api.send<T>(url, "PATCH", body),
  delete: <T>(url: string) => api.send<T>(url, "DELETE"),
  upload: <T>(url: string, file: File, field = "file") => api.uploadProgress<T>(url, file, undefined, field),
  uploadProgress: <T>(url: string, file: File, onProgress?: (pct: number, speed: number) => void, field = "file", signal?: AbortSignal) => {
    const body = new FormData();
    body.append(field, file);
    return new Promise<T>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const started = performance.now();
      let settled = false;
      const finish = () => signal?.removeEventListener("abort", stop);
      const ok = (data: T) => {
        if (settled) return;
        settled = true;
        finish();
        resolve(data);
      };
      const fail = (err: Error) => {
        if (settled) return;
        settled = true;
        finish();
        reject(err);
      };
      const stop = () => xhr.abort();
      xhr.open("POST", url);
      xhr.withCredentials = true;
      xhr.upload.onprogress = (ev) => {
        if (settled || !onProgress || !ev.lengthComputable || ev.total <= 0) return;
        onProgress(Math.min(100, Math.round((ev.loaded / ev.total) * 100)), uploadSpeed(ev.loaded, started, performance.now()));
      };
      xhr.onload = () => {
        if (xhr.status === 0) {
          fail(new UploadCancelled());
          return;
        }
        let data: ApiError = { error: "" };
        try {
          data = JSON.parse(xhr.responseText || "{}") as ApiError;
        } catch {
          data = { error: xhr.statusText };
        }
        if (xhr.status >= 200 && xhr.status < 300) {
          ok(data as T);
          return;
        }
        if (redirectIfSessionExpired(xhr.status, url)) return;
        fail(new RequestError(data.error || xhr.statusText || "Upload failed"));
      };
      xhr.onerror = () => fail(new RequestError("Upload failed"));
      xhr.onabort = () => fail(new UploadCancelled());
      if (signal) {
        if (signal.aborted) {
          fail(new UploadCancelled());
          return;
        }
        signal.addEventListener("abort", stop);
      }
      xhr.send(body);
    });
  },
};
