import api, { ApiResponse } from "@/lib/api";

type AuthStatusResponse = {
  authenticated?: boolean;
};

export async function checkAdminAuthenticated(): Promise<boolean> {
  try {
    const payload = await api.get<AuthStatusResponse>("/auth/status", {
      hideToast: true,
    });
    return payload?.authenticated === true;
  } catch {
    return false;
  }
}

export async function loginWithAdminToken(token: string): Promise<void> {
  const cleanToken = token.trim();
  if (!cleanToken) {
    throw new Error("请输入 token");
  }

  try {
    await api.post<{ ok?: boolean }>(
      "/auth/login",
      { token: cleanToken },
      { hideToast: true, errMsg: "登录失败" },
    );
  } catch (err) {
    const message = (err as ApiResponse<unknown>)?.msg || "登录失败";
    throw new Error(message);
  }
}
