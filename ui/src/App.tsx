import { BrowserRouter, Routes, Route, NavLink } from "react-router-dom";
import { LayoutDashboard, History } from "lucide-react";
import { AppsPage } from "@/pages/AppsPage";
import { AppPage } from "@/pages/AppPage";
import { CreateAppPage } from "@/pages/CreateAppPage";
import { RollbacksPage } from "@/pages/RollbacksPage";
import { BuildPage } from "@/pages/BuildPage";

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-background">
      <nav className="border-b bg-card">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex h-16 items-center justify-between">
            <div className="flex items-center gap-2">
              <LayoutDashboard className="h-6 w-6" />
              <span className="text-xl font-bold">TinyCloud</span>
            </div>
            <div className="flex items-center gap-6">
              <NavLink
                to="/apps"
                className={({ isActive }) =>
                  `flex items-center gap-2 text-sm font-medium transition-colors ${
                    isActive
                      ? "text-primary"
                      : "text-muted-foreground hover:text-primary"
                  }`
                }
              >
                <LayoutDashboard className="h-4 w-4" />
                Apps
              </NavLink>
              <NavLink
                to="/rollbacks"
                className={({ isActive }) =>
                  `flex items-center gap-2 text-sm font-medium transition-colors ${
                    isActive
                      ? "text-primary"
                      : "text-muted-foreground hover:text-primary"
                  }`
                }
              >
                <History className="h-4 w-4" />
                Rollbacks
              </NavLink>
            </div>
          </div>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {children}
      </main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/apps" element={<AppsPage />} />
          <Route path="/apps/new" element={<CreateAppPage />} />
          <Route path="/apps/:name" element={<AppPage />} />
          <Route path="/builds/:id" element={<BuildPage />} />
          <Route path="/rollbacks" element={<RollbacksPage />} />
          <Route path="/" element={<AppsPage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}
