import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import Navbar from "./components/navbar";
import Footer from "./components/footer";
import HomePage from "./pages/HomePage";
import WorkerListPage from "./pages/WorkerListPage";
import WorkerDetailPage from "./pages/WorkerDetailPage";
import DependenciesPage from "./pages/DependenciesPage";
import ConfigManagePage from "./pages/ConfigManagePage";
import AboutPage from "./pages/AboutPage";
import { checkAdminAuthenticated } from "./lib/admin-auth";
import { BASE_PREFIX } from "./main";

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<"loading" | "authenticated" | "guest">("loading");

  useEffect(() => {
    let active = true;

    const checkAuth = async () => {
      const authenticated = await checkAdminAuthenticated().catch(() => false);
      if (!active) return;
      setStatus(authenticated ? "authenticated" : "guest");
    };

    void checkAuth();

    return () => {
      active = false;
    };
  }, []);

  if (status === "loading") {
    return <section className="py-10 text-sm text-default-500">正在校验登录状态...</section>;
  }
  if (status === "guest") {
    return <Navigate replace to={BASE_PREFIX} />;
  }
  return children;
}

function App() {
  const className = "container mx-auto box-border max-w-7xl px-3 md:px-6";
  const style = { minHeight: "var(--main-height)", paddingTop: "102px" };
  const PREFIX = BASE_PREFIX;

  return (<>
    {<Navbar />}
    <main 
      className={className}
      style={style}
    >
      <Routes>
        <Route path={`${PREFIX}/`} element={<HomePage />} />
        <Route path={`${PREFIX}/workers`} element={<ProtectedRoute><WorkerListPage /></ProtectedRoute>} />
        <Route path={`${PREFIX}/workers/:id`} element={<ProtectedRoute><WorkerDetailPage /></ProtectedRoute>} />
        <Route path={`${PREFIX}/dependencies`} element={<ProtectedRoute><DependenciesPage /></ProtectedRoute>} />
        <Route path={`${PREFIX}/config`} element={<ProtectedRoute><ConfigManagePage /></ProtectedRoute>} />
        <Route path={`${PREFIX}/about`} element={<AboutPage />} />
        <Route path={`${PREFIX}/*`} element={<HomePage />} />
      </Routes>
    </main>
    {<Footer />}
  </>);
}

export default App;
