export type ApiError = { error: string };

export type Captcha = { id: string; image: string };

export class RequestError extends Error {
  captchaRequired?: boolean;
  captcha?: Captcha;
}

async function parse<T>(res: Response): Promise<T> {
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const payload = data as ApiError & { captchaRequired?: boolean; captcha?: Captcha };
    const err = new RequestError(payload.error || res.statusText);
    err.captchaRequired = payload.captchaRequired;
    err.captcha = payload.captcha;
    throw err;
  }
  return data as T;
}

export const api = {
  get: <T>(url: string) => fetch(url, { credentials: "include" }).then(parse<T>),
  send: <T>(url: string, method: string, body?: unknown) =>
    fetch(url, {
      method,
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    }).then(parse<T>),
  post: <T>(url: string, body?: unknown) => api.send<T>(url, "POST", body),
  put: <T>(url: string, body?: unknown) => api.send<T>(url, "PUT", body),
  patch: <T>(url: string, body?: unknown) => api.send<T>(url, "PATCH", body),
  delete: <T>(url: string) => api.send<T>(url, "DELETE"),
  upload: <T>(url: string, file: File, field = "file") => {
    const body = new FormData();
    body.append(field, file);
    return fetch(url, { method: "POST", credentials: "include", body }).then(parse<T>);
  },
};
