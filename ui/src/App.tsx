import { BrowserRouter, Routes, Route, NavLink, useLocation } from "react-router-dom";
import { useEffect, useState } from "react";
import { Activity, Boxes, Hammer, History, Server, BookOpen, Sun, Moon, LogOut, LogIn } from "lucide-react";
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

const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api";

/**
 * The signed-in identity, from the header oauth2-proxy sets after GitHub login.
 * The browser cannot read that header itself, so the API reflects it back.
 *
 * "No session" is a state this has to report, not swallow. It used to set user
 * to null on any failure and render the full console anyway — so an expired
 * cookie, or a tab left open past the session lifetime, produced a shell with a
 * Sign out button and nothing anywhere to sign back in with. The only way back
 * was to know to clear the cookie by hand.
 */
type Auth = "checking" | "in" | "out";

function useIdentity() {
  const [user, setUser] = useState<string | null>(null);
  const [auth, setAuth] = useState<Auth>("checking");
  const [signOutUrl, setSignOutUrl] = useState("/oauth2/sign_out?rd=/");

  useEffect(() => {
    let cancelled = false;
    fetch(`${API_BASE}/v1/me`, { credentials: "include" })
      .then(async (r) => {
        if (cancelled) return;
        // oauth2-proxy answers an unauthenticated request with its own sign-in
        // page and a 401/403 — so a non-ok response here means no session,
        // not a broken API.
        if (r.status === 401 || r.status === 403) {
          setAuth("out");
          return;
        }
        if (!r.ok) {
          // Something else is wrong (API down, 502). Don't claim the user is
          // signed out and send them through a login they don't need.
          setAuth("in");
          return;
        }
        const b = await r.json();
        const d = b.data ?? b;
        setUser(d.user ?? null);
        if (d.signOutUrl) setSignOutUrl(d.signOutUrl);
        setAuth("in");
      })
      .catch(() => {
        if (!cancelled) setAuth("in");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { user, auth, signOutUrl };
}

/**
 * Shown when there is no session. Returns to whatever page was being viewed:
 * oauth2-proxy takes the target as ?rd= and validates it against
 * --whitelist-domain, so a relative path is both accepted and safe.
 */
function SignIn() {
  const loc = useLocation();
  const rd = encodeURIComponent(loc.pathname + loc.search);

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--color-bg)] px-4">
      <div className="rise w-full max-w-sm rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] p-6">
        <span className="font-mono text-sm font-semibold tracking-tight text-[var(--color-accent)]">
          tinycloud
        </span>
        <h1 className="mt-3 font-mono text-base text-[var(--color-ink)]">Session ended</h1>
        <p className="mt-1.5 text-sm leading-relaxed text-[var(--color-muted)]">
          The ops console is restricted to a single GitHub account. Sign in to continue.
        </p>
        <a
          href={`/oauth2/start?rd=${rd}`}
          className="mt-5 flex items-center justify-center gap-2 rounded-[var(--radius-sm)] bg-[var(--color-accent)] px-3 py-2 font-mono text-xs font-medium text-[var(--color-bg)] transition-opacity hover:opacity-90"
        >
          <LogIn className="h-4 w-4" />
          Sign in with GitHub
        </a>
      </div>
    </div>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  const { user, auth, signOutUrl } = useIdentity();
  const { effective, toggle } = useTheme();
  const loc = useLocation();

  // Nothing is rendered until the session is known: showing the console and
  // then replacing it with a login screen reads as a crash.
  if (auth === "checking") return <div className="min-h-screen bg-[var(--color-bg)]" />;
  if (auth === "out") return <SignIn />;

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

          <div className="ml-auto flex items-center gap-1">
            {user && (
              <span className="hidden font-mono text-[11px] text-[var(--color-muted)] sm:inline">
                {user}
              </span>
            )}

            <button
              onClick={toggle}
              aria-label={`Switch to ${effective === "dark" ? "light" : "dark"} theme`}
              className="rounded-[var(--radius-sm)] p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-ink)]"
            >
              {effective === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>

            {/* A link, not a fetch: oauth2-proxy handles this path itself and
                needs to clear the session cookie and redirect the browser. */}
            <a
              href={signOutUrl}
              aria-label="Sign out"
              title={user ? `Sign out of ${user}` : "Sign out"}
              className="rounded-[var(--radius-sm)] p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-crit)]"
            >
              <LogOut className="h-4 w-4" />
            </a>
          </div>
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
