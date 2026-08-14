import { FormEvent, useEffect, useState } from "react";
import { Button } from "@heroui/react";
import { useNavigate } from "react-router-dom";
import { BASE_PREFIX } from "@/main";
import { Input } from "@/components/heroui";

import { checkAdminAuthenticated, loginWithAdminToken } from "@/lib/admin-auth";
import { useTranslation } from "react-i18next";

export default function HomePage() {
  const { t } = useTranslation();
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
        navigate(`${BASE_PREFIX}/workers`, { replace: true });
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
      navigate(`${BASE_PREFIX}/workers`, { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : t("auth.failed");
      setError(message);
    } finally {
      setSubmitting(false);
    }
  };

  if (checkingAuth) {
    return (
      <section className="flex justify-center items-center h-full mt-10">
        <div className="w-full max-w-md rounded-2xl border border-default-200 px-6 py-8 shadow-sm">
          <p className="text-sm text-default-500">{t("auth.checking")}</p>
        </div>
      </section>
    );
  }

  return (
    <section className="flex justify-center items-center h-full mt-10">
      <div className="w-full max-w-md rounded-2xl border border-default-200 px-6 py-8 shadow-sm bg-background/70">
        <h1 className="text-2xl font-semibold text-default-900">{t("auth.title")}</h1>
        <p className="mt-2 text-sm text-default-500">{t("auth.hint")}</p>

        <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
          <Input
            autoComplete="current-password"
            isRequired
            label="Token"
            name="token"
            placeholder={t("auth.tokenPlaceholder")}
            type="password"
            value={token}
            onValueChange={setToken}
          />
          {error && <p className="text-sm text-danger">{error}</p>}
          <Button
            className="w-full"
            variant="primary"
            isDisabled={token.trim() === "" || submitting}
            isPending={submitting}
            type="submit"
          >
            {t("auth.login")}
          </Button>
        </form>
      </div>
    </section>
  );
}
