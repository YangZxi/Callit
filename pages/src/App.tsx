import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import Navbar from "./components/navbar";
import Footer from "./components/footer";
import HomePage from "./pages/HomePage";
import WorkerListPage from "./pages/WorkerListPage";
import WorkerDetailPage from "./pages/WorkerDetailPage";
import AboutPage from "./pages/AboutPage";
import DependenciesPage from "./pages/DependenciesPage";
import { checkAdminAuthenticated } from "./lib/admin-auth";

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
    return <Navigate replace to="/" />;
  }
  return children;
}

function App() {
  const className = "container mx-auto box-border max-w-7xl px-3 md:px-6";
  const style = { minHeight: "var(--main-height)", paddingTop: "36px" };

  return (<>
    {<Navbar />}
    <main 
      className={className}
      style={style}
    >
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/workers" element={<ProtectedRoute><WorkerListPage /></ProtectedRoute>} />
        <Route path="/workers/:id" element={<ProtectedRoute><WorkerDetailPage /></ProtectedRoute>} />
        <Route path="/dependencies" element={<ProtectedRoute><DependenciesPage /></ProtectedRoute>} />
        <Route path="/about" element={<AboutPage />} />
        <Route path="*" element={<HomePage />} />
      </Routes>
    </main>
    {<Footer />}
  </>);
}

export default App;
