/// <reference types="vite/client" />
import { addToast } from "@heroui/react";
import { BASE_PREFIX } from "@/main";

export const API_BASE = `${BASE_PREFIX}/api`;

const toastCache = new Set<string>();

type HttpMethod = "GET" | "POST" | "PUT" | "DELETE";

export type ApiResponse<T> = {
  msg: string;
  data: T;
  code: number;
};

type options = {
  hideToast?: boolean;
  errMsg?: string;
}

async function request<T>(method: HttpMethod, url: string, body?: unknown, options?: options): Promise<T> {
  const target = url.startsWith("http") ? url : `${API_BASE}${url}`;
  const isFormData = body instanceof FormData;
  const headers: HeadersInit = {};
  if (!isFormData) {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(target, {
    method,
    headers,
    credentials: "include",
    body: body ? (isFormData ? body : JSON.stringify(body)) : undefined,
  });

  const json = (await res.json().catch((err) => {
    console.error("Failed to parse JSON response:", err);
    return null;
  })) as ApiResponse<T> | null;
  if (!json || typeof json.code !== "number" || json.code !== 200) {
    if (!(options?.hideToast === true)) {
      toast(options?.errMsg || json?.msg);
    }
    return Promise.reject(json as ApiResponse<T>);
  }
  return json.data;
}

function toast(msg?: string) {
  const text = msg || "请求失败";
  if (toastCache.has(text)) return;
  toastCache.add(text);

  const timeout = 3000;
  addToast({
    title: text,
    color: "danger",
    variant: "flat",
    timeout: timeout,
  });
  // toast 消失后允许再次弹出
  setTimeout(() => {
    toastCache.delete(text);
  }, timeout);
}

const api = {
  get: <T>(url: string, options?: options) => request<T>("GET", url, undefined, options),
  put: <T>(url: string, body?: unknown, options?: options) => request<T>("PUT", url, body, options),
  delete: <T>(url: string, options?: options) => request<T>("DELETE", url, undefined, options),
  post: <T>(url: string, body?: unknown, options?: options) => request<T>("POST", url, body, options),
};

export default api;
