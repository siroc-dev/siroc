export type ApiError = { error: string };

async function parse<T>(res: Response): Promise<T> {
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as ApiError).error || res.statusText);
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
};
