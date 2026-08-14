import api, { ApiResponse } from "@/lib/api";
import i18n from "@/i18n";

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
    throw new Error(i18n.t("api.tokenRequired"));
  }

  try {
    await api.post<{ ok?: boolean }>(
      "/auth/login",
      { token: cleanToken },
      { hideToast: true, errMsg: i18n.t("auth.failed") },
    );
  } catch (err) {
    const message = (err as ApiResponse<unknown>)?.msg || i18n.t("auth.failed");
    throw new Error(message);
  }
}
