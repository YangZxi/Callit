import { FormEvent, useEffect, useState } from "react";
import { Button, Input } from "@heroui/react";
import { useNavigate } from "react-router-dom";

import { checkAdminAuthenticated, loginWithAdminToken } from "@/lib/admin-auth";

export default function HomePage() {
  const navigate = useNavigate();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let active = true;

    const checkAuth = async () => {
      const authenticated = await checkAdminAuthenticated().catch(() => false);
      if (!active) return;
      if (authenticated) {
        navigate("/workers", { replace: true });
        return;
      }
      setCheckingAuth(false);
    };

    void checkAuth();

    return () => {
      active = false;
    };
  }, [navigate]);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;

    setSubmitting(true);
    setError("");
    try {
      await loginWithAdminToken(token);
      navigate("/workers", { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : "登录失败";
      setError(message);
    } finally {
      setSubmitting(false);
    }
  };

  if (checkingAuth) {
    return (
      <section className="flex justify-center items-center h-full mt-10">
        <div className="w-full max-w-md rounded-2xl border border-default-200 px-6 py-8 shadow-sm">
          <p className="text-sm text-default-500">正在检查登录状态...</p>
        </div>
      </section>
    );
  }

  return (
    <section className="flex justify-center items-center h-full mt-10">
      <div className="w-full max-w-md rounded-2xl border border-default-200 px-6 py-8 shadow-sm bg-background/70">
        <h1 className="text-2xl font-semibold text-default-900">Callit 管理后台登录</h1>
        <p className="mt-2 text-sm text-default-500">请输入启动服务时配置的 ADMIN_TOKEN</p>

        <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
          <Input
            autoComplete="current-password"
            isRequired
            label="Token"
            name="token"
            placeholder="请输入 ADMIN_TOKEN"
            type="password"
            value={token}
            onValueChange={setToken}
          />
          {error && <p className="text-sm text-danger">{error}</p>}
          <Button
            className="w-full"
            color="primary"
            isDisabled={token.trim() === "" || submitting}
            isLoading={submitting}
            type="submit"
          >
            登录
          </Button>
        </form>
      </div>
    </section>
  );
}
