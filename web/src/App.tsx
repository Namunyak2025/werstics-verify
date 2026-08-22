import {
  Activity,
  ArrowRight,
  BadgeCheck,
  BarChart3,
  Building2,
  Check,
  ChevronRight,
  CircleAlert,
  CircleDollarSign,
  CreditCard,
  Database,
  Fingerprint,
  KeyRound,
  LockKeyhole,
  LogOut,
  Plus,
  RefreshCw,
  ScanLine,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Users,
  X,
  Zap,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  clearToken,
  createPayment,
  getMe,
  getPayment,
  listAudit,
  listPayments,
  getToken,
  health,
  login,
  logout,
  submitVerification,
  type AuditRecord,
  type Payment,
  type User,
} from "./lib/api";

type View =
  | "overview"
  | "payments"
  | "verify"
  | "security"
  | "settings";


function formatMoney(currency: string, minor: number) {
  return `${currency} ${minor.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })}`;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en-KE", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function paymentStatusLabel(status: string) {
  switch (status.toLowerCase()) {
    case "requested":
      return "Requested";
    case "pending":
      return "Pending";
    case "confirmed":
      return "Confirmed";
    case "settled":
      return "Settled";
    case "failed":
      return "Failed";
    case "expired":
      return "Expired";
    case "reversed":
      return "Reversed";
    case "refunded":
      return "Refunded";
    case "cancelled":
      return "Cancelled";
    default:
      return status;
  }
}

function statusTone(status: string) {
  const normalized = status.toLowerCase();

  if (
    normalized.includes("confirm") ||
    normalized.includes("settle") ||
    normalized === "ok"
  ) {
    return "success";
  }

  if (
    normalized.includes("fail") ||
    normalized.includes("reject") ||
    normalized.includes("reverse") ||
    normalized.includes("refund")
  ) {
    return "danger";
  }

  return "pending";
}

function App() {
  const [view, setView] = useState<View>("overview");
  const [user, setUser] = useState<User | null>(null);
  const [permissions, setPermissions] = useState<string[]>([]);
  const [apiOnline, setApiOnline] = useState(false);
  const [booting, setBooting] = useState(true);

  const [loginForm, setLoginForm] = useState({
    organizationId: "11111111-1111-4111-8111-111111111111",
    email: "admin@werstics.local",
    password: "",
  });

  const [loginError, setLoginError] = useState("");
  const [toast, setToast] = useState("");

  const [payment, setPayment] = useState<Payment | null>(null);
  const [paymentHistory, setPaymentHistory] = useState<Payment[]>([]);
  const [paymentTotal, setPaymentTotal] = useState(0);
  const [paymentPage, setPaymentPage] = useState(1);
  const [paymentLoading, setPaymentLoading] = useState(false);
  const [auditRecords, setAuditRecords] =
    useState<AuditRecord[]>([]);
  const [auditTotal, setAuditTotal] = useState(0);
  const [auditPage, setAuditPage] = useState(1);
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditSearch, setAuditSearch] = useState("");
  const [auditAction, setAuditAction] = useState("");
  const [auditActorType, setAuditActorType] = useState("");
  const [paymentLookup, setPaymentLookup] = useState("");

  const [paymentForm, setPaymentForm] = useState({
    merchantId: "merchant_console",
    provider: "simulator",
    providerRef: "",
    currency: "KES",
    amount: "1500",
    customer: "",
  });

  const [verificationForm, setVerificationForm] = useState({
    paymentId: "",
    provider: "simulator",
    providerEventId: "",
    providerRef: "",
    merchantId: "merchant_console",
    currency: "KES",
    amount: "1500",
    kind: "payment.confirmed",
  });

  const [verificationResult, setVerificationResult] =
    useState<Awaited<ReturnType<typeof submitVerification>> | null>(
      null,
    );

  const notify = (message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(""), 2800);
  };

  const loadIdentity = async () => {
    const [identity, system] = await Promise.all([
      getMe(),
      health(),
    ]);

    setUser(identity.user);
    setPermissions(identity.permissions);
    setApiOnline(system.status === "ok");

    return identity;
  };

  useEffect(() => {
    const boot = async () => {
      try {
        if (!getToken()) {
          setBooting(false);
          return;
        }

        const identity = await loadIdentity();

        if (identity.permissions.includes("payment:read")) {
          await loadPayments(1, true);
        }
      } catch {
        clearToken();
        setUser(null);
        setPermissions([]);
      } finally {
        setBooting(false);
      }
    };

    void boot();
  }, []);

  const canCreate = permissions.includes("payment:create");
  const canRead = permissions.includes("payment:read");
  const canVerify = permissions.includes("payment:verify");
  const canAuditRead =
    permissions.includes("audit:read");
  async function loadPayments(
    page = 1,
    allowed = canRead,
  ) {
    if (!allowed) {
      return;
    }

    setPaymentLoading(true);

    try {
      const result = await listPayments({
        page,
        page_size: 25,
      });

      setPaymentHistory(result.payments);
      setPaymentTotal(result.total);
      setPaymentPage(result.page);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Unable to load payments.",
      );
    } finally {
      setPaymentLoading(false);
    }  }


  const permissionCount = permissions.length;

  const groupedPermissions = useMemo(
    () => ({
      payment: permissions.filter((p) => p.startsWith("payment:")),
      identity: permissions.filter(
        (p) =>
          p.startsWith("user:") ||
          p.startsWith("role:"),
      ),
      organization: permissions.filter((p) =>
        p.startsWith("organization:"),
      ),
      audit: permissions.filter((p) => p.startsWith("audit:")),
    }),
    [permissions],
  );

  async function loadAudit(
    page = 1,
    allowed = canAuditRead,
  ) {
    if (!allowed) {
      return;
    }

    setAuditLoading(true);

    try {
      const result = await listAudit({
        page,
        page_size: 25,
        action: auditAction || undefined,
        actor_type: auditActorType || undefined,
        search: auditSearch || undefined,
      });

      setAuditRecords(result.records);
      setAuditTotal(result.total);
      setAuditPage(result.page);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Unable to load security activity.",
      );
    } finally {
      setAuditLoading(false);
    }
  }

  async function applyAuditFilters() {
    await loadAudit(1, true);
  }

  async function clearAuditFilters() {
    setAuditSearch("");
    setAuditAction("");
    setAuditActorType("");
    await loadAudit(1, true);
  }

  const submitLogin = async (
    event: React.FormEvent,
  ) => {
    event.preventDefault();
    setLoginError("");

    try {
      await login(
        loginForm.organizationId,
        loginForm.email,
        loginForm.password,
      );

      const identity = await loadIdentity();

      if (identity.permissions.includes("payment:read")) {
        await loadPayments(1, true);
      }

      setLoginForm((current) => ({
        ...current,
        password: "",
      }));

      notify("Signed in");
    } catch (error) {
      setLoginError(
        error instanceof Error
          ? error.message
          : "Unable to sign in.",
      );
    }
  };

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      setUser(null);
      setPermissions([]);
      notify("Signed out");
    }
  };

  const handleCreatePayment = async (
    event: React.FormEvent,
  ) => {
    event.preventDefault();

    if (!user || !canCreate) {
      notify("Your account cannot create payments.");
      return;
    }

    try {
      const created = await createPayment({
        organization_id: user.organization_id,
        merchant_id: paymentForm.merchantId,
        provider: paymentForm.provider,
        provider_ref: paymentForm.providerRef,
        expected: {
          currency: paymentForm.currency.toUpperCase(),
          minor: Number(paymentForm.amount),
        },
        customer_display: paymentForm.customer,
      });

      setPayment(created);
      setPaymentHistory((current) => [
        created,
        ...current.filter((item) => item.id !== created.id),
      ]);
      setPaymentLookup(created.id);
      setVerificationForm((current) => ({
        ...current,
        paymentId: created.id,
        providerRef: created.provider_ref ?? "",
        merchantId: created.merchant_id,
        currency: created.expected.currency,
        amount: String(created.expected.minor),
      }));

      notify("Payment created");
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Payment creation failed.",
      );
    }
  };

  const selectPayment = (selected: Payment) => {
    setPayment(selected);
    setPaymentLookup(selected.id);

    setVerificationForm((current) => ({
      ...current,
      paymentId: selected.id,
      providerRef: selected.provider_ref ?? "",
      merchantId: selected.merchant_id,
      currency: selected.expected.currency,
      amount: String(selected.expected.minor),
    }));
  };

  const handleLookupDirect = async (paymentId: string) => {
    if (!paymentId.trim() || !canRead) {
      notify("Enter a payment ID and ensure payment read access.");
      return;
    }

    try {
      const result = await getPayment(paymentId.trim());

      selectPayment(result);

      setPaymentHistory((current) => [
        result,
        ...current.filter((item) => item.id !== result.id),
      ]);

      notify("Payment state refreshed");
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Payment lookup failed.",
      );
    }
  };

  const handleVerification = async (
    event: React.FormEvent,
  ) => {
    event.preventDefault();

    if (!canVerify) {
      notify("Your account cannot verify payment events.");
      return;
    }

    try {
      const result = await submitVerification(
        verificationForm.paymentId,
        {
          provider: verificationForm.provider,
          provider_event_id:
            verificationForm.providerEventId,
          provider_ref: verificationForm.providerRef,
          merchant_id: verificationForm.merchantId,
          amount: {
            currency: verificationForm.currency.toUpperCase(),
            minor: Number(verificationForm.amount),
          },
          kind: verificationForm.kind,
        },
      );

      setVerificationResult(result);
      setPayment(result.payment ?? null);
      notify("Provider event processed");
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Verification failed.",
      );
    }
  };

  if (booting) {
    return (
      <div className="loading-screen">
        <div className="loading-mark">W</div>
        <div>
          <strong>Werstics Verify</strong>
          <span>Restoring secure workspace…</span>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="auth-page">
        <div className="auth-orbit auth-orbit-a" />
        <div className="auth-orbit auth-orbit-b" />

        <div className="auth-layout">
          <section className="auth-story">
            <div className="product-lockup">
              <div className="brand-emblem">W</div>
              <div>
                <strong>WERSTICS VERIFY</strong>
                <span>Payment verification infrastructure</span>
              </div>
            </div>

            <div className="story-content">
              <div className="eyebrow">Operational trust layer</div>
              <h1>
                Know when a payment
                <em> actually happened.</em>
              </h1>

              <p>
                A controlled verification environment for payment
                events, organization access, and deterministic
                authorization.
              </p>

              <div className="story-signals">
                <div>
                  <ShieldCheck size={18} />
                  <span>Organization-scoped access</span>
                </div>
                <div>
                  <Fingerprint size={18} />
                  <span>Identity-aware authorization</span>
                </div>
                <div>
                  <ScanLine size={18} />
                  <span>Deterministic verification core</span>
                </div>
              </div>
            </div>

            <div className="auth-footnote">
              Werstics Verify does not infer payment truth from
              screenshots, SMS text, or customer claims.
            </div>
          </section>

          <section className="auth-panel">
            <div className="panel-kicker">
              <span className="status-dot" />
              Secure console
            </div>

            <h2>Sign in to your workspace</h2>
            <p className="muted">
              Your organization credentials stay within the Verify
              control plane.
            </p>

            <form className="auth-form" onSubmit={submitLogin}>
              <label>
                Organization ID
                <input
                  value={loginForm.organizationId}
                  onChange={(event) =>
                    setLoginForm((current) => ({
                      ...current,
                      organizationId:
                        event.target.value,
                    }))
                  }
                />
              </label>

              <label>
                Email
                <input
                  type="email"
                  value={loginForm.email}
                  onChange={(event) =>
                    setLoginForm((current) => ({
                      ...current,
                      email: event.target.value,
                    }))
                  }
                />
              </label>

              <label>
                Password
                <input
                  type="password"
                  value={loginForm.password}
                  onChange={(event) =>
                    setLoginForm((current) => ({
                      ...current,
                      password: event.target.value,
                    }))
                  }
                  autoComplete="current-password"
                />
              </label>

              {loginError && (
                <div className="error-banner">
                  <CircleAlert size={17} />
                  <span>{loginError}</span>
                </div>
              )}

              <button className="button-primary button-large">
                Enter Verify
                <ArrowRight size={17} />
              </button>
            </form>

            <div className="auth-security-strip">
              <LockKeyhole size={16} />
              <span>Session protected by server-side authorization</span>
            </div>
          </section>
        </div>
      </div>
    );
  }

  return (
    <div className="console-shell">
      <aside className="sidebar">
        <div className="sidebar-header">
          <div className="brand-emblem small">W</div>
          <div>
            <strong>Werstics</strong>
            <span>Verify</span>
          </div>
        </div>

        <div className="workspace-chip">
          <Building2 size={15} />
          <div>
            <span>Workspace</span>
            <strong>
              {user.display_name || "Organization"}
            </strong>
          </div>
        </div>

        <nav className="nav">
          <button
            className={view === "overview" ? "active" : ""}
            onClick={() => setView("overview")}
          >
            <BarChart3 size={18} />
            <span>Overview</span>
          </button>

          <button
            className={view === "payments" ? "active" : ""}
            onClick={() => setView("payments")}
          >
            <CreditCard size={18} />
            <span>Payments</span>
            {canRead && <ChevronRight size={14} />}
          </button>

          <button
            className={view === "verify" ? "active" : ""}
            onClick={() => setView("verify")}
          >
            <ScanLine size={18} />
            <span>Verify</span>
            {canVerify && <ChevronRight size={14} />}
          </button>

          <button
            className={view === "security" ? "active" : ""}
            onClick={() => {
              setView("security");

              if (canAuditRead) {
                void loadAudit(1, true);
              }
            }}
          >
            <ShieldCheck size={18} />
            <span>Security</span>
          </button>

          <button
            className={view === "settings" ? "active" : ""}
            onClick={() => setView("settings")}
          >
            <SlidersHorizontal size={18} />
            <span>Settings</span>
          </button>
        </nav>

        <div className="sidebar-lower">
          <div className="system-card">
            <div className="system-card-head">
              <Activity size={16} />
              <span>System state</span>
            </div>
            <div className="system-state">
              <span className="status-dot" />
              <strong>
                {apiOnline ? "Operational" : "Degraded"}
              </strong>
            </div>
            <small>
              API + authorization path responding
            </small>
          </div>

          <button
            className="sidebar-user"
            onClick={() => setView("settings")}
          >
            <div className="avatar">
              {user.display_name.slice(0, 1).toUpperCase()}
            </div>
            <div>
              <strong>{user.display_name}</strong>
              <span>{user.email}</span>
            </div>
            <ChevronRight size={15} />
          </button>
        </div>
      </aside>

      <main className="main">
        <header className="topbar">
          <div>
            <span className="topbar-eyebrow">
              {view === "overview" && "Operational overview"}
              {view === "payments" && "Payment operations"}
              {view === "verify" && "Deterministic verification"}
              {view === "security" && "Security boundary"}
              {view === "settings" && "Workspace configuration"}
            </span>

            <h2>
              {view === "overview" && "Command center"}
              {view === "payments" && "Payment workspace"}
              {view === "verify" && "Verification desk"}
              {view === "security" && "Identity & access"}
              {view === "settings" && "Workspace settings"}
            </h2>
          </div>

          <div className="topbar-actions">
            <div className="live-status">
              <span className="status-dot" />
              {apiOnline ? "Connected" : "Offline"}
            </div>

            <button
              className="icon-button"
              onClick={() => void loadIdentity()}
              title="Refresh"
            >
              <RefreshCw size={17} />
            </button>

            <button
              className="logout-button"
              onClick={() => void handleLogout()}
            >
              <LogOut size={16} />
              Sign out
            </button>
          </div>
        </header>

        <div className="page">
          {view === "overview" && (
            <>
              <section className="hero-card">
                <div className="hero-card-copy">
                  <div className="eyebrow">Werstics Verify</div>
                  <h1>
                    Payment truth,
                    <br />
                    <em>without the noise.</em>
                  </h1>
                  <p>
                    Your workspace is authenticated, organization-scoped,
                    and connected to the deterministic verification
                    infrastructure.
                  </p>

                  <div className="hero-actions">
                    <button
                      className="button-primary"
                      onClick={() => setView("payments")}
                    >
                      <Plus size={17} />
                      Create payment
                    </button>

                    <button
                      className="button-secondary"
                      onClick={() => setView("verify")}
                    >
                      Open verification desk
                      <ArrowRight size={16} />
                    </button>
                  </div>
                </div>

                <div className="hero-orbit">
                  <div className="orbit-ring orbit-ring-a" />
                  <div className="orbit-ring orbit-ring-b" />
                  <div className="orbit-core">
                    <BadgeCheck size={35} />
                    <span>Verified</span>
                  </div>
                </div>
              </section>

              <section className="signal-grid">
                <article className="signal-card dark">
                  <div className="signal-icon orange">
                    <Fingerprint size={19} />
                  </div>
                  <span>Identity</span>
                  <strong>Authenticated</strong>
                  <small>{user.email}</small>
                </article>

                <article className="signal-card warm">
                  <div className="signal-icon teal">
                    <KeyRound size={19} />
                  </div>
                  <span>Authorization</span>
                  <strong>{permissionCount} permissions</strong>
                  <small>
                    {groupedPermissions.payment.length} payment capabilities
                  </small>
                </article>

                <article className="signal-card blue">
                  <div className="signal-icon blue">
                    <Database size={19} />
                  </div>
                  <span>Persistence</span>
                  <strong>PostgreSQL</strong>
                  <small>Transaction-backed state</small>
                </article>

                <article className="signal-card green">
                  <div className="signal-icon green">
                    <Zap size={19} />
                  </div>
                  <span>Verification core</span>
                  <strong>Deterministic</strong>
                  <small>Provider remains authoritative</small>
                </article>
              </section>

              <section className="overview-grid">
                <article className="panel-card">
                  <div className="section-header">
                    <div>
                      <span className="eyebrow">Access model</span>
                      <h3>What this workspace allows</h3>
                    </div>
                    <Users size={18} />
                  </div>

                  <div className="permission-groups">
                    <PermissionGroup
                      title="Payments"
                      permissions={groupedPermissions.payment}
                    />
                    <PermissionGroup
                      title="Organization"
                      permissions={groupedPermissions.organization}
                    />
                    <PermissionGroup
                      title="Identity"
                      permissions={groupedPermissions.identity}
                    />
                    <PermissionGroup
                      title="Audit"
                      permissions={groupedPermissions.audit}
                    />
                  </div>
                </article>

                <article className="panel-card activity-panel">
                  <div className="section-header">
                    <div>
                      <span className="eyebrow">Live control</span>
                      <h3>Current session</h3>
                    </div>
                    <Activity size={18} />
                  </div>

                  <div className="session-row">
                    <div className="session-marker">
                      <Fingerprint size={17} />
                    </div>
                    <div>
                      <strong>Authenticated session</strong>
                      <span>{user.email}</span>
                    </div>
                    <span className="session-state">Active</span>
                  </div>

                  <div className="session-row">
                    <div className="session-marker">
                      <Building2 size={17} />
                    </div>
                    <div>
                      <strong>Organization scope</strong>
                      <span>{user.organization_id}</span>
                    </div>
                    <span className="session-state">Bound</span>
                  </div>

                  <div className="session-row">
                    <div className="session-marker">
                      <LockKeyhole size={17} />
                    </div>
                    <div>
                      <strong>Authorization boundary</strong>
                      <span>Server enforced</span>
                    </div>
                    <span className="session-state">Enforced</span>
                  </div>
                </article>
              </section>
            </>
          )}

          {view === "payments" && (
            <section className="payments-workspace">
              <div className="workspace-heading">
                <div>
                  <span className="eyebrow">Payment operations</span>
                  <h1>Payment queue</h1>
                  <p>
                    Work from the payments already present in this
                    session. Select a payment to inspect its state or
                    continue into verification.
                  </p>
                </div>

                <button
                  className="button-primary"
                  onClick={() => setView("verify")}
                  disabled={!payment}
                >
                  Open verification
                  <ArrowRight size={16} />
                </button>
              </div>

              <div className="queue-toolbar">
                <div className="queue-search">
                  <Search size={16} />
                  <input
                    value={paymentLookup}
                    onChange={(event) =>
                      setPaymentLookup(event.target.value)
                    }
                    placeholder="Search payment ID..."
                  />
                </div>

                <div className="queue-metrics">
                  <span>{paymentTotal} total</span>

                  <span>
                    {paymentHistory.filter(
                      (item) => item.status.toLowerCase() === "requested",
                    ).length}{" "}
                    requested
                  </span>

                  <span>
                    {paymentHistory.filter(
                      (item) => item.status.toLowerCase() === "pending",
                    ).length}{" "}
                    pending
                  </span>

                  <span>
                    {paymentHistory.filter(
                      (item) => item.status.toLowerCase() === "confirmed",
                    ).length}{" "}
                    confirmed
                  </span>

                  <span>
                    {paymentHistory.filter((item) =>
                      ["failed", "expired", "reversed", "refunded", "cancelled"]
                        .includes(item.status.toLowerCase()),
                    ).length}{" "}
                    exceptional
                  </span>
                </div>

                <button
                  className="button-secondary compact"
                  onClick={() => void handleLookupDirect(paymentLookup)}
                  disabled={!canRead || !paymentLookup.trim()}
                >
                  Inspect
                </button>
              </div>

              <div className="queue-layout">
                <article className="queue-panel">
                  <div className="queue-panel-header">
                    <div>
                      <span className="eyebrow">Loaded activity</span>
                      <h3>Payments in this workspace</h3>
                    </div>
                    <CreditCard size={18} />
                  </div>

                  {paymentLoading ? (
                    <div className="empty-state queue-empty">
                      <RefreshCw size={28} className="spin" />
                      <strong>Loading payments</strong>
                      <span>
                        Reading the organization payment queue…
                      </span>
                    </div>
                  ) : paymentHistory.length === 0 ? (
                    <div className="empty-state queue-empty">
                      <CircleDollarSign size={30} />
                      <strong>No payments loaded yet</strong>
                      <span>
                        Create a payment or inspect an existing payment ID
                        to populate the operational queue.
                      </span>
                    </div>
                  ) : (
                    <div className="payment-list">
                      {paymentHistory.map((item) => (
                        <button
                          className={`payment-row ${
                            payment?.id === item.id ? "selected" : ""
                          }`}
                          key={item.id}
                          onClick={() => selectPayment(item)}
                        >
                          <div className="payment-state-mark">
                            <span
                              className={`state-dot ${statusTone(
                                item.status,
                              )}`}
                            />
                          </div>

                          <div className="payment-row-main">
                            <strong>{item.id}</strong>
                            <span>
                              {item.merchant_id} · {item.provider}
                            </span>
                          </div>

                          <div className="payment-row-amount">
                            <strong>
                              {formatMoney(
                                item.expected.currency,
                                item.expected.minor,
                              )}
                            </strong>
                            <span>{paymentStatusLabel(item.status)}</span>
                          </div>

                          <ChevronRight size={16} />
                        </button>
                      ))}
                    </div>
                  )}

                  {paymentTotal > 25 && (
                    <div className="queue-pagination">
                      <span>
                        Page {paymentPage} · {paymentTotal} payments
                      </span>

                      <div>
                        <button
                          className="button-secondary compact"
                          disabled={
                            paymentLoading || paymentPage <= 1
                          }
                          onClick={() =>
                            void loadPayments(paymentPage - 1, true)
                          }
                        >
                          Previous
                        </button>

                        <button
                          className="button-secondary compact"
                          disabled={
                            paymentLoading ||
                            paymentPage * 25 >= paymentTotal
                          }
                          onClick={() =>
                            void loadPayments(paymentPage + 1, true)
                          }
                        >
                          Next
                        </button>
                      </div>
                    </div>
                  )}
                </article>

                <article className="panel-card payment-inspector">
                  <div className="queue-panel-header">
                    <div>
                      <span className="eyebrow">Selected payment</span>
                      <h3>
                        {payment ? payment.id : "Nothing selected"}
                      </h3>
                    </div>
                    {payment && (
                      <span
                        className={`status-pill ${statusTone(
                          payment.status,
                        )}`}
                      >
                        {paymentStatusLabel(payment.status)}
                      </span>
                    )}
                  </div>

                  {payment ? (
                    <>
                      <div className="inspector-amount">
                        <span>Expected amount</span>
                        <strong>
                          {formatMoney(
                            payment.expected.currency,
                            payment.expected.minor,
                          )}
                        </strong>
                      </div>

                      <div className="detail-list">
                        <div>
                          <span>Merchant</span>
                          <strong>{payment.merchant_id}</strong>
                        </div>
                        <div>
                          <span>Provider</span>
                          <strong>{payment.provider}</strong>
                        </div>
                        <div>
                          <span>Provider reference</span>
                          <strong>
                            {payment.provider_ref || "Not supplied"}
                          </strong>
                        </div>
                        <div>
                          <span>Customer</span>
                          <strong>
                            {payment.customer_display ||
                              "Not supplied"}
                          </strong>
                        </div>
                        <div>
                          <span>Created</span>
                          <strong>{formatDate(payment.created_at)}</strong>
                        </div>
                      </div>

                      <div className="inspector-actions">
                        <button
                          className="button-primary"
                          onClick={() => setView("verify")}
                          disabled={!canVerify}
                        >
                          <ScanLine size={16} />
                          Verify payment
                        </button>

                        <button
                          className="button-secondary"
                          onClick={() => void handleLookupDirect(payment.id)}
                          disabled={!canRead || paymentLoading}
                        >
                          <RefreshCw size={16} />
                          Refresh state
                        </button>
                      </div>
                    </>
                  ) : (
                    <div className="empty-state">
                      <Search size={28} />
                      <strong>Select a payment</strong>
                      <span>
                        The inspector will show state, amount, merchant,
                        and provider context.
                      </span>
                    </div>
                  )}
                </article>
              </div>

              <article className="panel-card create-strip">
                <div>
                  <span className="eyebrow">New payment</span>
                  <h3>Create a controlled payment request</h3>
                  <p>
                    Creation still goes through the existing
                    organization-scoped API and RBAC boundary.
                  </p>
                </div>

                <form
                  className="create-strip-form"
                  onSubmit={handleCreatePayment}
                >
                  <input
                    value={paymentForm.merchantId}
                    onChange={(event) =>
                      setPaymentForm((current) => ({
                        ...current,
                        merchantId: event.target.value,
                      }))
                    }
                    placeholder="Merchant ID"
                    disabled={!canCreate}
                  />

                  <input
                    value={paymentForm.providerRef}
                    onChange={(event) =>
                      setPaymentForm((current) => ({
                        ...current,
                        providerRef: event.target.value,
                      }))
                    }
                    placeholder="Provider reference"
                    disabled={!canCreate}
                  />

                  <input
                    value={paymentForm.amount}
                    onChange={(event) =>
                      setPaymentForm((current) => ({
                        ...current,
                        amount: event.target.value,
                      }))
                    }
                    type="number"
                    min="1"
                    placeholder="Minor amount"
                    disabled={!canCreate}
                  />

                  <button
                    className="button-primary"
                    disabled={!canCreate}
                  >
                    <Plus size={16} />
                    Create
                  </button>
                </form>
              </article>
            </section>
          )}

          {view === "verify" && (
            <section className="verify-workspace">
              <div className="workspace-heading">
                <div>
                  <span className="eyebrow">
                    Deterministic verification
                  </span>
                  <h1>Verification desk</h1>
                  <p>
                    Investigate one persisted payment at a time, supply
                    the provider event, and read the decision produced
                    by the verification core.
                  </p>
                </div>

                <div className="verify-session-badge">
                  <span className="status-dot" />
                  {payment
                    ? `Investigating ${payment.id}`
                    : "No payment selected"}
                </div>
              </div>

              <div className="verify-layout">
                <article className="panel-card verify-context">
                  <div className="queue-panel-header">
                    <div>
                      <span className="eyebrow">Payment context</span>
                      <h3>
                        {payment?.id || "Select a payment"}
                      </h3>
                    </div>
                    <Fingerprint size={18} />
                  </div>

                  {payment ? (
                    <>
                      <div className="verification-context-hero">
                        <div className="context-state-line">
                          <span
                            className={`status-pill ${statusTone(
                              payment.status,
                            )}`}
                          >
                            {paymentStatusLabel(payment.status)}
                          </span>

                          <span className="context-provider">
                            {payment.provider}
                          </span>
                        </div>

                        <span>Expected amount</span>

                        <strong>
                          {formatMoney(
                            payment.expected.currency,
                            payment.expected.minor,
                          )}
                        </strong>

                        <small>
                          {payment.merchant_id}
                          {" · "}
                          {payment.customer_display ||
                            "No customer label"}
                        </small>
                      </div>

                      <div className="verification-facts">
                        <div>
                          <span>Payment ID</span>
                          <strong>{payment.id}</strong>
                        </div>

                        <div>
                          <span>Status</span>
                          <strong>
                            {paymentStatusLabel(payment.status)}
                          </strong>
                        </div>

                        <div>
                          <span>Provider</span>
                          <strong>{payment.provider}</strong>
                        </div>

                        <div>
                          <span>Provider reference</span>
                          <strong>
                            {payment.provider_ref ||
                              "Not supplied"}
                          </strong>
                        </div>

                        <div>
                          <span>Merchant</span>
                          <strong>{payment.merchant_id}</strong>
                        </div>

                        <div>
                          <span>Session</span>
                          <strong>{payment.session_id}</strong>
                        </div>

                        <div>
                          <span>Organization</span>
                          <strong>
                            {payment.organization_id}
                          </strong>
                        </div>

                        <div>
                          <span>Created</span>
                          <strong>
                            {formatDate(payment.created_at)}
                          </strong>
                        </div>
                      </div>

                      <div className="verification-context-note">
                        <ShieldCheck size={15} />
                        <span>
                          The decision is evaluated against the
                          persisted payment context: organization,
                          merchant, provider, expected amount, and
                          payment state.
                        </span>
                      </div>
                    </>
                  ) : (
                    <div className="empty-state">
                      <CreditCard size={28} />
                      <strong>No payment selected</strong>
                      <span>
                        Select a payment from the Payments queue to
                        begin verification.
                      </span>
                    </div>
                  )}
                </article>

                <article className="panel-card verify-input">
                  <div className="queue-panel-header">
                    <div>
                      <span className="eyebrow">
                        Provider event
                      </span>
                      <h3>Event intake</h3>
                      <span className="verify-target">
                        Target:{" "}
                        {payment?.id ||
                          verificationForm.paymentId ||
                          "not selected"}
                      </span>
                    </div>
                    <Zap size={18} />
                  </div>

                  {!canVerify && (
                    <div className="notice danger">
                      <CircleAlert size={17} />
                      <span>
                        Your role does not include payment:verify.
                      </span>
                    </div>
                  )}

                  {payment && (
                    <div className="verify-target-strip">
                      <div className="target-icon">
                        <ScanLine size={16} />
                      </div>

                      <div>
                        <strong>
                          Verifying {payment.id}
                        </strong>
                        <span>
                          Expected{" "}
                          {formatMoney(
                            payment.expected.currency,
                            payment.expected.minor,
                          )}{" "}
                          from {payment.merchant_id}
                        </span>
                      </div>

                      <span
                        className={`status-pill ${statusTone(
                          payment.status,
                        )}`}
                      >
                        {paymentStatusLabel(payment.status)}
                      </span>
                    </div>
                  )}

                  <form
                    className="console-form"
                    onSubmit={handleVerification}
                  >
                    <label>
                      Payment ID
                      <input
                        value={verificationForm.paymentId}
                        onChange={(event) =>
                          setVerificationForm((current) => ({
                            ...current,
                            paymentId: event.target.value,
                          }))
                        }
                        disabled={!canVerify}
                      />
                    </label>

                    <div className="field-grid">
                      <label>
                        Event kind
                        <select
                          value={verificationForm.kind}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              kind: event.target.value,
                            }))
                          }
                          disabled={!canVerify}
                        >
                          <option value="payment.pending">
                            payment.pending
                          </option>
                          <option value="payment.confirmed">
                            payment.confirmed
                          </option>
                          <option value="payment.settled">
                            payment.settled
                          </option>
                          <option value="payment.failed">
                            payment.failed
                          </option>
                          <option value="payment.expired">
                            payment.expired
                          </option>
                          <option value="payment.reversed">
                            payment.reversed
                          </option>
                          <option value="payment.refunded">
                            payment.refunded
                          </option>
                          <option value="payment.cancelled">
                            payment.cancelled
                          </option>
                        </select>
                      </label>

                      <label>
                        Provider
                        <input
                          value={verificationForm.provider}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              provider: event.target.value,
                            }))
                          }
                          disabled={!canVerify}
                        />
                      </label>
                    </div>

                    <label>
                      Provider event ID
                      <input
                        value={verificationForm.providerEventId}
                        onChange={(event) =>
                          setVerificationForm((current) => ({
                            ...current,
                            providerEventId:
                              event.target.value,
                          }))
                        }
                        disabled={!canVerify}
                      />
                    </label>

                    <div className="field-grid">
                      <label>
                        Provider reference
                        <input
                          value={verificationForm.providerRef}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              providerRef:
                                event.target.value,
                            }))
                          }
                          disabled={!canVerify}
                        />
                      </label>

                      <label>
                        Merchant ID
                        <input
                          value={verificationForm.merchantId}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              merchantId:
                                event.target.value,
                            }))
                          }
                          disabled={!canVerify}
                        />
                      </label>
                    </div>

                    <div className="field-grid">
                      <label>
                        Currency
                        <input
                          maxLength={3}
                          value={verificationForm.currency}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              currency:
                                event.target.value.toUpperCase(),
                            }))
                          }
                          disabled={!canVerify}
                        />
                      </label>

                      <label>
                        Minor amount
                        <input
                          type="number"
                          min="1"
                          value={verificationForm.amount}
                          onChange={(event) =>
                            setVerificationForm((current) => ({
                              ...current,
                              amount: event.target.value,
                            }))
                          }
                          disabled={!canVerify}
                        />
                      </label>
                    </div>

                    <button
                      className="button-primary"
                      disabled={
                        !canVerify ||
                        !verificationForm.paymentId
                      }
                    >
                      <ScanLine size={16} />
                      Run deterministic verification
                    </button>
                  </form>
                </article>
              </div>

              <article className="panel-card verification-timeline">
                <div className="queue-panel-header">
                  <div>
                    <span className="eyebrow">
                      Decision trail
                    </span>
                    <h3>
                      {payment
                        ? `Verification analysis · ${payment.id}`
                        : "Verification analysis"}
                    </h3>
                  </div>
                  <Activity size={18} />
                </div>

                {verificationResult?.match ? (
                  <div className="analysis-grid">
                    <div
                      className={`decision-hero ${
                        verificationResult.match.matched
                          ? "verified"
                          : "rejected"
                      }`}
                    >
                      {verificationResult.match.matched ? (
                        <Check size={25} />
                      ) : (
                        <X size={25} />
                      )}

                      <div>
                        <span>Final decision</span>
                        <strong>
                          {verificationResult.match.matched
                            ? "MATCHED"
                            : "MISMATCH"}
                        </strong>

                        <p>
                          {verificationResult.match.reason ||
                            "Decision returned by the deterministic verification core."}
                        </p>

                        {payment && (
                          <small className="decision-payment-id">
                            {payment.id}
                          </small>
                        )}
                      </div>
                    </div>

                    <div className="analysis-track">
                      <DecisionStep
                        label="Amount"
                        passed={
                          verificationResult.match
                            .amount_matched
                        }
                      />

                      <DecisionStep
                        label="Merchant"
                        passed={
                          verificationResult.match
                            .merchant_matched
                        }
                      />

                      <DecisionStep
                        label="Provider event"
                        passed={Boolean(
                          verificationResult.payment,
                        )}
                      />

                      <DecisionStep
                        label="Payment state"
                        text={
                          verificationResult.payment?.status ??
                          "Unavailable"
                        }
                      />
                    </div>
                  </div>
                ) : (
                  <div className="empty-state tall">
                    <ScanLine size={31} />
                    <strong>
                      {payment
                        ? "Ready to evaluate this payment"
                        : "Decision trail is empty"}
                    </strong>
                    <span>
                      {payment
                        ? "Submit the provider event above to produce the deterministic decision."
                        : "Select a payment from the Payments queue to begin."}
                    </span>
                  </div>
                )}
              </article>
            </section>
          )}

          {view === "security" && (
            <section className="security-workspace">
              <div className="workspace-heading">
                <div>
                  <span className="eyebrow">Security activity</span>
                  <h1>Audit stream</h1>
                  <p>
                    Review the organization-scoped security and operational
                    events recorded by Werstics Verify.
                  </p>
                </div>

                <div className="security-header-actions">
                  <div className="decision-badge">
                    <span className="status-dot" />
                    {canAuditRead
                      ? `${auditTotal} recorded events`
                      : "Audit access restricted"}
                  </div>

                  <button
                    className="icon-button"
                    onClick={() => void loadAudit(auditPage, true)}
                    disabled={!canAuditRead || auditLoading}
                    title="Refresh audit stream"
                  >
                    <RefreshCw size={16} />
                  </button>
                </div>
              </div>

              {!canAuditRead ? (
                <article className="panel-card security-denied">
                  <LockKeyhole size={30} />
                  <strong>Audit access is restricted</strong>
                  <span>
                    Your role does not include audit:read, so security
                    activity remains unavailable to this principal.
                  </span>
                </article>
              ) : (
                <>
                  <article className="audit-filter-panel">
                    <div className="audit-filter-search">
                      <Search size={16} />
                      <input
                        value={auditSearch}
                        onChange={(event) =>
                          setAuditSearch(event.target.value)
                        }
                        placeholder="Search actions or resources..."
                      />
                    </div>

                    <select
                      value={auditAction}
                      onChange={(event) =>
                        setAuditAction(event.target.value)
                      }
                    >
                      <option value="">All actions</option>
                      <option value="auth.login">auth.login</option>
                      <option value="auth.logout">auth.logout</option>
                      <option value="payment.list">payment.list</option>
                      <option value="payment.created">
                        payment.created
                      </option>
                      <option value="payment.verification_completed">
                        payment.verification_completed
                      </option>
                      <option value="payment.verification_failed">
                        payment.verification_failed
                      </option>
                      <option value="payment.verification_denied">
                        payment.verification_denied
                      </option>
                      <option value="audit.list">audit.list</option>
                    </select>

                    <select
                      value={auditActorType}
                      onChange={(event) =>
                        setAuditActorType(event.target.value)
                      }
                    >
                      <option value="">All actors</option>
                      <option value="user">User</option>
                      <option value="system">System</option>
                    </select>

                    <button
                      className="button-primary compact"
                      onClick={() => void applyAuditFilters()}
                      disabled={auditLoading}
                    >
                      Apply
                    </button>

                    <button
                      className="button-secondary compact"
                      onClick={() => void clearAuditFilters()}
                      disabled={auditLoading}
                    >
                      Clear
                    </button>
                  </article>

                  <article className="panel-card audit-stream-panel">
                    <div className="section-header">
                      <div>
                        <span className="eyebrow">
                          Recorded activity
                        </span>
                        <h3>Security and operational events</h3>
                      </div>
                      <Activity size={18} />
                    </div>

                    {auditLoading ? (
                      <div className="empty-state tall">
                        <RefreshCw size={30} className="spin" />
                        <strong>Reading audit stream</strong>
                        <span>
                          Loading organization-scoped events...
                        </span>
                      </div>
                    ) : auditRecords.length === 0 ? (
                      <div className="empty-state tall">
                        <ShieldCheck size={30} />
                        <strong>No matching events</strong>
                        <span>
                          Try a broader search or remove the selected
                          filters.
                        </span>
                      </div>
                    ) : (
                      <div className="audit-stream">
                        {auditRecords.map((record) => (
                          <AuditRow
                            key={record.ID}
                            record={record}
                          />
                        ))}
                      </div>
                    )}

                    {auditTotal > 25 && (
                      <div className="queue-pagination">
                        <span>
                          Page {auditPage} - {auditTotal} events
                        </span>

                        <div>
                          <button
                            className="button-secondary compact"
                            disabled={
                              auditLoading ||
                              auditPage <= 1
                            }
                            onClick={() =>
                              void loadAudit(
                                auditPage - 1,
                                true,
                              )
                            }
                          >
                            Previous
                          </button>

                          <button
                            className="button-secondary compact"
                            disabled={
                              auditLoading ||
                              auditPage * 25 >= auditTotal
                            }
                            onClick={() =>
                              void loadAudit(
                                auditPage + 1,
                                true,
                              )
                            }
                          >
                            Next
                          </button>
                        </div>
                      </div>
                    )}
                  </article>
                </>
              )}
            </section>
          )}

          {view === "settings" && (
            <section className="settings-layout">
              <article className="panel-card">
                <div className="section-header">
                  <div>
                    <span className="eyebrow">Workspace</span>
                    <h3>Organization context</h3>
                  </div>
                  <Building2 size={18} />
                </div>

                <div className="detail-list large">
                  <div>
                    <span>Organization ID</span>
                    <strong>{user.organization_id}</strong>
                  </div>
                  <div>
                    <span>Principal</span>
                    <strong>{user.display_name}</strong>
                  </div>
                  <div>
                    <span>Email</span>
                    <strong>{user.email}</strong>
                  </div>
                </div>
              </article>

              <article className="panel-card">
                <div className="section-header">
                  <div>
                    <span className="eyebrow">Capabilities</span>
                    <h3>Granted permissions</h3>
                  </div>
                  <LockKeyhole size={18} />
                </div>

                <div className="chips large-chips">
                  {permissions.map((permission) => (
                    <span className="chip" key={permission}>
                      {permission}
                    </span>
                  ))}
                </div>
              </article>
            </section>
          )}
        </div>
      </main>

      {toast && (
        <div className="toast">
          <Sparkles size={15} />
          <span>{toast}</span>
        </div>
      )}
    </div>
  );
}

function AuditRow({
  record,
}: {
  record: AuditRecord;
}) {
  const metadata = record.Metadata || {};

  const matched =
    typeof metadata.matched === "boolean"
      ? metadata.matched
      : undefined;

  const reason =
    typeof metadata.reason === "string"
      ? metadata.reason
      : "";

  const provider =
    typeof metadata.provider === "string"
      ? metadata.provider
      : "";

  const email =
    typeof metadata.email === "string"
      ? metadata.email
      : "";

  const total =
    typeof metadata.total === "number"
      ? metadata.total
      : undefined;

  const actionLabel = record.Action
    .replaceAll(".", " - ")
    .replaceAll("_", " ");

  const resource =
    record.ResourceID || record.ResourceType;

  const actor =
    record.ActorType === "system"
      ? "System"
      : email || record.ActorUserID || "User";

  return (
    <div className="audit-row">
      <div
        className={`audit-marker ${
          matched === false
            ? "danger"
            : matched === true
              ? "success"
              : "neutral"
        }`}
      >
        {matched === false ? (
          <X size={16} />
        ) : matched === true ? (
          <Check size={16} />
        ) : (
          <Activity size={16} />
        )}
      </div>

      <div className="audit-row-main">
        <div className="audit-row-top">
          <strong>{actionLabel}</strong>
          <span className="audit-time">
            {formatDate(record.CreatedAt)}
          </span>
        </div>

        <div className="audit-row-resource">
          <span>{resource}</span>
          <span className="audit-separator">-</span>
          <span>{actor}</span>

          {provider && (
            <>
              <span className="audit-separator">-</span>
              <span>{provider}</span>
            </>
          )}
        </div>

        {(reason || total !== undefined) && (
          <div className="audit-row-detail">
            {reason && <span>{reason}</span>}
            {total !== undefined && (
              <span>{total} records returned</span>
            )}
          </div>
        )}
      </div>

      <span
        className={`audit-actor-chip ${record.ActorType}`}
      >
        {record.ActorType}
      </span>
    </div>
  );
}

function DecisionStep({
  label,
  passed,
  text,
}: {
  label: string;
  passed?: boolean;
  text?: string;
}) {
  return (
    <div className="decision-step">
      <div
        className={`decision-step-icon ${
          typeof passed === "boolean"
            ? passed
              ? "passed"
              : "failed"
            : "neutral"
        }`}
      >
        {typeof passed === "boolean" ? (
          passed ? (
            <Check size={15} />
          ) : (
            <X size={15} />
          )
        ) : (
          <Activity size={15} />
        )}
      </div>

      <div>
        <span>{label}</span>
        <strong>
          {typeof passed === "boolean"
            ? passed
              ? "Matched"
              : "Mismatch"
            : text}
        </strong>
      </div>
    </div>
  );
}

function PermissionGroup({
  title,
  permissions,
}: {
  title: string;
  permissions: string[];
}) {
  return (
    <div className="permission-group">
      <span>{title}</span>
      <div>
        {permissions.length === 0 ? (
          <small className="muted">No granted capabilities</small>
        ) : (
          permissions.map((permission) => (
            <span className="permission-chip" key={permission}>
              {permission}
            </span>
          ))
        )}
      </div>
    </div>
  );
}

export default App;
