import { BrowserRouter, Routes, Route, NavLink, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { Activity, Boxes, Hammer, History, Server, BookOpen, Sun, Moon } from "lucide-react";
import { OverviewPage } from "@/pages/OverviewPage";
import { AppsPage } from "@/pages/AppsPage";
import { AppPage } from "@/pages/AppPage";
import { CreateAppPage } from "@/pages/CreateAppPage";
import { RollbacksPage } from "@/pages/RollbacksPage";
import { BuildPage } from "@/pages/BuildPage";
import { BuildsPage } from "@/pages/BuildsPage";
import { InfraPage } from "@/pages/InfraPage";
import { SystemPage } from "@/pages/SystemPage";

const NAV = [
  { to: "/", label: "Overview", icon: Activity, end: true },
  { to: "/apps", label: "Apps", icon: Boxes, end: false },
  { to: "/builds", label: "Builds", icon: Hammer, end: false },
  { to: "/infra", label: "Infra", icon: Server, end: false },
  { to: "/rollbacks", label: "Rollbacks", icon: History, end: false },
  { to: "/system", label: "System", icon: BookOpen, end: false },
];

/**
 * Theme. Defaults to the OS preference and only writes data-theme once the
 * user chooses, so "system" stays a real third state rather than being
 * collapsed into whichever value we guessed at first paint.
 */
function useTheme() {
  const [theme, setTheme] = useState<"dark" | "light" | null>(() => {
    const s = localStorage.getItem("tinycloud-theme");
    return s === "dark" || s === "light" ? s : null;
  });

  useEffect(() => {
    const root = document.documentElement;
    if (theme) {
      root.setAttribute("data-theme", theme);
      localStorage.setItem("tinycloud-theme", theme);
    } else {
      root.removeAttribute("data-theme");
      localStorage.removeItem("tinycloud-theme");
    }
  }, [theme]);

  const prefersDark = window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
  const effective = theme ?? (prefersDark ? "dark" : "light");
  return { effective, toggle: () => setTheme(effective === "dark" ? "light" : "dark") };
}

function Shell({ children }: { children: React.ReactNode }) {
  const { effective, toggle } = useTheme();
  const loc = useLocation();

  return (
    <div className="min-h-screen bg-[var(--color-bg)]">
      <header className="sticky top-0 z-20 border-b border-[var(--color-line)] bg-[var(--color-bg)]/85 backdrop-blur">
        <div className="mx-auto flex h-12 max-w-[1400px] items-center gap-6 px-4">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm font-semibold tracking-tight text-[var(--color-accent)]">
              tinycloud
            </span>
          </div>

          <nav className="flex items-center gap-0.5 overflow-x-auto">
            {NAV.map(({ to, label, icon: Icon, end }) => (
              <NavLink
                key={to}
                to={to}
                end={end}
                className={({ isActive }) =>
                  `flex items-center gap-1.5 rounded-[var(--radius-sm)] px-2.5 py-1 font-mono text-xs transition-colors ${
                    isActive
                      ? "bg-[var(--color-surface-2)] text-[var(--color-ink)]"
                      : "text-[var(--color-muted)] hover:bg-[var(--color-surface)] hover:text-[var(--color-ink-2)]"
                  }`
                }
              >
                <Icon className="h-3.5 w-3.5" />
                {label}
              </NavLink>
            ))}
          </nav>

          <button
            onClick={toggle}
            aria-label={`Switch to ${effective === "dark" ? "light" : "dark"} theme`}
            className="ml-auto rounded-[var(--radius-sm)] p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-ink)]"
          >
            {effective === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
        </div>
      </header>

      <main key={loc.pathname} className="rise mx-auto max-w-[1400px] px-4 py-6">
        {children}
      </main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/apps" element={<AppsPage />} />
          <Route path="/apps/new" element={<CreateAppPage />} />
          <Route path="/apps/:name" element={<AppPage />} />
          <Route path="/builds" element={<BuildsPage />} />
          <Route path="/builds/:id" element={<BuildPage />} />
          <Route path="/build/:id" element={<BuildPage />} />
          <Route path="/infra" element={<InfraPage />} />
          <Route path="/rollbacks" element={<RollbacksPage />} />
          <Route path="/system" element={<SystemPage />} />
        </Routes>
      </Shell>
    </BrowserRouter>
  );
}
