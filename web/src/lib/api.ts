export type User = {
  id: string;
  organization_id: string;
  email: string;
  display_name: string;
  status: string;
};

export type Money = {
  currency: string;
  minor: number;
};

export type Payment = {
  id: string;
  organization_id: string;
  merchant_id: string;
  session_id: string;
  provider: string;
  provider_ref?: string;
  expected: Money;
  customer_display?: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type MatchResult = {
  matched?: boolean;
  amount_matched?: boolean;
  merchant_matched?: boolean;
  reason?: string;
};

export type VerificationResponse = {
  payment?: Payment;
  match?: MatchResult;
};

const TOKEN_KEY = "werstics_verify_token";

export const getToken = () =>
  sessionStorage.getItem(TOKEN_KEY) ?? "";

export const setToken = (token: string) =>
  sessionStorage.setItem(TOKEN_KEY, token);

export const clearToken = () =>
  sessionStorage.removeItem(TOKEN_KEY);

async function request<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);

  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const token = getToken();

  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(path, {
    ...init,
    headers,
  });

  const text = await response.text();

  let data: unknown = null;

  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }

  if (!response.ok) {
    const message =
      typeof data === "string"
        ? data
        : typeof data === "object" &&
            data !== null &&
            "message" in data
          ? String((data as { message?: unknown }).message)
          : `Request failed (${response.status})`;

    throw new Error(message);
  }

  return data as T;
}

export async function login(
  organization_id: string,
  email: string,
  password: string,
) {
  const result = await request<{
    token: string;
    user: User;
  }>("/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({
      organization_id,
      email,
      password,
    }),
  });

  setToken(result.token);

  return result;
}

export function logout() {
  return request("/v1/auth/logout", {
    method: "POST",
  }).finally(() => {
    clearToken();
  });
}

export function getMe() {
  return request<{
    user: User;
    permissions: string[];
  }>("/v1/auth/me");
}

export function health() {
  return request<{
    status: string;
    service: string;
  }>("/health");
}


export type AuditRecord = {
  ID: string;
  OrganizationID: string;
  ActorUserID: string;
  ActorType: string;
  Action: string;
  ResourceType: string;
  ResourceID: string;
  Metadata: Record<string, unknown>;
  CreatedAt: string;
};

export type AuditListResponse = {
  records: AuditRecord[];
  page: number;
  page_size: number;
  total: number;
};

export function listAudit(params: {
  page?: number;
  page_size?: number;
  action?: string;
  resource_type?: string;
  resource_id?: string;
  actor_type?: string;
  search?: string;
} = {}) {
  const query = new URLSearchParams();

  if (params.page) query.set("page", String(params.page));
  if (params.page_size) {
    query.set("page_size", String(params.page_size));
  }
  if (params.action) query.set("action", params.action);
  if (params.resource_type) {
    query.set("resource_type", params.resource_type);
  }
  if (params.resource_id) {
    query.set("resource_id", params.resource_id);
  }
  if (params.actor_type) {
    query.set("actor_type", params.actor_type);
  }
  if (params.search) query.set("search", params.search);

  const suffix = query.toString()
    ? `?${query.toString()}`
    : "";

  return request<AuditListResponse>(
    `/v1/audit${suffix}`,
  );
}

export function createPayment(input: {
  organization_id: string;
  merchant_id: string;
  provider: string;
  provider_ref?: string;
  expected: Money;
  customer_display?: string;
}) {
  return request<Payment>("/v1/payments", {
    method: "POST",
    body: JSON.stringify({
      id: `pay_console_${Date.now()}`,
      organization_id: input.organization_id,
      merchant_id: input.merchant_id,
      session_id: `session_console_${Date.now()}`,
      provider: input.provider,
      provider_ref: input.provider_ref,
      expected: input.expected,
      customer_display: input.customer_display,
    }),
  });
}

export function getPayment(paymentId: string) {
  return request<Payment>(
    `/v1/payments/${encodeURIComponent(paymentId)}`,
  );
}

export type PaymentListResponse = {
  payments: Payment[];
  page: number;
  page_size: number;
  total: number;
};

export function listPayments(params: {
  page?: number;
  page_size?: number;
  status?: string;
  provider?: string;
  merchant_id?: string;
  provider_ref?: string;
  search?: string;
} = {}) {
  const query = new URLSearchParams();

  if (params.page) query.set("page", String(params.page));
  if (params.page_size) query.set("page_size", String(params.page_size));
  if (params.status) query.set("status", params.status);
  if (params.provider) query.set("provider", params.provider);
  if (params.merchant_id) query.set("merchant_id", params.merchant_id);
  if (params.provider_ref) query.set("provider_ref", params.provider_ref);
  if (params.search) query.set("search", params.search);

  const suffix = query.toString() ? `?${query.toString()}` : "";

  return request<PaymentListResponse>(
    `/v1/payments${suffix}`,
  );
}

export function submitVerification(
  paymentId: string,
  input: {
    provider: string;
    provider_event_id: string;
    provider_ref: string;
    merchant_id: string;
    amount: Money;
    kind: string;
  },
) {
  return request<VerificationResponse>(
    `/v1/payments/${encodeURIComponent(paymentId)}/events`,
    {
      method: "POST",
      body: JSON.stringify({
        event_id: `evt_console_${Date.now()}`,
        provider: input.provider,
        provider_event_id: input.provider_event_id,
        provider_ref: input.provider_ref,
        merchant_id: input.merchant_id,
        amount: input.amount,
        kind: input.kind,
        occurred_at: new Date().toISOString(),
      }),
    },
  );
}
