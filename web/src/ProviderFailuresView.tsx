import {
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import { useEffect, useState } from "react";
import {
  listProviderFailures,
  resolveProviderFailure,
  retryProviderFailure,
  type ProviderEventFailure,
} from "./lib/api";

function formatDate(value: string) {
  return new Intl.DateTimeFormat("en-KE", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatMoney(currency: string, minor: number) {
  return `${currency} ${minor.toLocaleString("en-KE")}`;
}

type Props = {
  canVerify: boolean;
  notify: (message: string) => void;
};

export default function ProviderFailuresView({
  canVerify,
  notify,
}: Props) {
  const [items, setItems] = useState<ProviderEventFailure[]>([]);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState("retryable");
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState("");

  async function load() {
    if (!canVerify) {
      setItems([]);
      setTotal(0);
      setLoading(false);
      return;
    }

    setLoading(true);

    try {
      const result = await listProviderFailures({
        page: 1,
        page_size: 50,
        status,
      });

      setItems(result.failures);
      setTotal(result.total);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Unable to load provider failures.",
      );
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, [status, canVerify]);

  async function retry(id: string) {
    setBusyId(id);

    try {
      const result = await retryProviderFailure(id);
      notify(
        `Recovery completed for ${result.payment_id}.`,
      );
      await load();
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Provider failure retry failed.",
      );
    } finally {
      setBusyId("");
    }
  }

  async function resolve(id: string) {
    setBusyId(id);

    try {
      await resolveProviderFailure(id);
      notify("Provider failure marked resolved.");
      await load();
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Provider failure resolution failed.",
      );
    } finally {
      setBusyId("");
    }
  }

  if (!canVerify) {
    return (
      <section className="security-workspace">
        <div className="workspace-heading">
          <div>
            <span className="eyebrow">Operational recovery</span>
            <h1>Provider failures</h1>
            <p>
              Retry and resolution controls are restricted to
              principals with payment:verify.
            </p>
          </div>
        </div>

        <article className="panel-card security-denied">
          <ShieldCheck size={30} />
          <strong>Recovery access is restricted</strong>
          <span>
            Your organization role does not include payment:verify.
          </span>
        </article>
      </section>
    );
  }

  return (
    <section className="security-workspace">
      <div className="workspace-heading">
        <div>
          <span className="eyebrow">Operational recovery</span>
          <h1>Provider failures</h1>
          <p>
            Investigate provider-processing failures without
            weakening the deterministic payment core.
          </p>
        </div>

        <div className="security-header-actions">
          <div className="decision-badge">
            <span className="status-dot" />
            {total} {status} failures
          </div>

          <button
            className="icon-button"
            onClick={() => void load()}
            disabled={loading}
            title="Refresh failures"
          >
            <RefreshCw size={16} />
          </button>
        </div>
      </div>

      <article className="audit-filter-panel">
        <select
          value={status}
          onChange={(event) => setStatus(event.target.value)}
        >
          <option value="retryable">Retryable</option>
          <option value="resolved">Resolved</option>
        </select>
      </article>

      <article className="panel-card audit-stream-panel">
        <div className="section-header">
          <div>
            <span className="eyebrow">Failure ledger</span>
            <h3>Provider processing queue</h3>
          </div>
          <AlertTriangle size={18} />
        </div>

        {loading ? (
          <div className="empty-state tall">
            <RefreshCw size={30} className="spin" />
            <strong>Loading provider failures</strong>
            <span>
              Reading organization-scoped recovery records...
            </span>
          </div>
        ) : items.length === 0 ? (
          <div className="empty-state tall">
            <CheckCircle2 size={30} />
            <strong>No matching provider failures</strong>
            <span>
              The recovery queue is clear for this status.
            </span>
          </div>
        ) : (
          <div className="audit-stream">
            {items.map((failure) => {
              const busy = busyId === failure.id;
              const resolved = failure.status === "resolved";

              return (
                <div className="audit-row" key={failure.id}>
                  <div
                    className={`audit-marker ${
                      resolved ? "success" : "danger"
                    }`}
                  >
                    {resolved ? (
                      <CheckCircle2 size={16} />
                    ) : (
                      <CircleAlert size={16} />
                    )}
                  </div>

                  <div className="audit-row-main">
                    <div className="audit-row-top">
                      <strong>
                        {failure.provider} · {failure.kind}
                      </strong>
                      <span className="audit-time">
                        {formatDate(failure.last_failed_at)}
                      </span>
                    </div>

                    <div className="audit-row-resource">
                      <span>{failure.provider_event_id}</span>
                      <span className="audit-separator">·</span>
                      <span>{failure.payment_id}</span>
                      <span className="audit-separator">·</span>
                      <span>
                        {formatMoney(
                          failure.amount_currency,
                          failure.amount_minor,
                        )}
                      </span>
                    </div>

                    <div className="audit-row-detail">
                      <span>
                        Attempt {failure.attempts}
                      </span>
                      <span>
                        {failure.last_error}
                      </span>
                    </div>

                    <div className="inspector-actions">
                      {!resolved && (
                        <>
                          <button
                            className="button-primary compact"
                            disabled={busy}
                            onClick={() =>
                              void retry(failure.id)
                            }
                          >
                            <RotateCcw size={15} />
                            {busy ? "Retrying..." : "Retry"}
                          </button>

                          <button
                            className="button-secondary compact"
                            disabled={busy}
                            onClick={() =>
                              void resolve(failure.id)
                            }
                          >
                            <XCircle size={15} />
                            Resolve
                          </button>
                        </>
                      )}

                      <span className="audit-actor-chip system">
                        {failure.status}
                      </span>
                    </div>
                  </div>

                  <ChevronRight size={16} />
                </div>
              );
            })}
          </div>
        )}
      </article>
    </section>
  );
}
