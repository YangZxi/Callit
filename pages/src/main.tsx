import React, { useEffect } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { ToastProvider } from "@heroui/react";

import App from "./App.tsx";
import "@/styles/globals.css";


function AppShell({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const updateAppHeight = () => {
      // 使用真实可视高度，避免移动端地址栏导致 100vh 误差
      document.documentElement.style.setProperty(
        "--app-height",
        `${window.innerHeight}px`,
      );
    };

    updateAppHeight();
    window.addEventListener("resize", updateAppHeight);
    window.addEventListener("orientationchange", updateAppHeight);

    return () => {
      window.removeEventListener("resize", updateAppHeight);
      window.removeEventListener("orientationchange", updateAppHeight);
    };
  }, []);

  return <div
    className="w-full min-h-screen theme-wrapper"
    style={{
      backgroundImage: `url()`,
      backgroundSize: "cover",
      backgroundRepeat: "no-repeat",
      backgroundPosition: "center",
      backgroundAttachment: "fixed",
      minHeight: "var(--app-height)",
    }}
  >
    <ToastProvider placement="top" width={260} />
    {children}
  </div>;
}

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <BrowserRouter>
      <AppShell>
        <App />
      </AppShell>
    </BrowserRouter>
  </React.StrictMode>,
);
