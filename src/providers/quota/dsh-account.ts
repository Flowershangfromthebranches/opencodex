import { DSH_PLATFORM_ORIGIN } from "../../oauth/dsh";
import { asRecord, QUOTA_JSON_READ_FAILURE, readQuotaJson, REQUEST_TIMEOUT_MS } from "../quota-wire";
import type { ProviderQuota, ProviderQuotaWindow } from "../quota-types";
import { report, TERMINAL_QUOTA_FAILURE, type ProviderQuotaProbeResult } from "./report-cache";

function formatMoney(valStr: unknown, currStr: unknown): string {
  const num = typeof valStr === "number" ? valStr : parseFloat(String(valStr ?? "0"));
  const currency = String(currStr ?? "CNY").trim().toUpperCase();
  const sign = currency === "CNY" ? "¥" : currency === "USD" ? "$" : `${currency} `;
  return `${sign}${Number.isFinite(num) ? num.toFixed(2) : "0.00"}`;
}

export async function fetchDshAccountQuota(
  provider: string,
  accessToken: string,
): Promise<ProviderQuotaProbeResult> {
  if (!accessToken || !accessToken.trim()) return null;

  try {
    const response = await fetch(`${DSH_PLATFORM_ORIGIN}/api/v0/users/get_user_summary`, {
      method: "GET",
      headers: {
        "x-dsh-auth-token": accessToken.trim(),
        "Accept": "application/json",
      },
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });

    if (response.status === 401) {
      return TERMINAL_QUOTA_FAILURE;
    }
    if (!response.ok) {
      return null;
    }

    const bodyRaw = await readQuotaJson(response);
    if (bodyRaw === QUOTA_JSON_READ_FAILURE) {
      return null;
    }

    const body = asRecord(bodyRaw);
    if (!body || body.code !== 0) {
      return null;
    }

    const data = asRecord(body.data);
    const bizData = asRecord(data?.biz_data);
    const summary = asRecord(bizData?.user_summary) ?? bizData;
    if (!summary) return null;

    const normalWallets = Array.isArray(summary.normal_wallets) ? summary.normal_wallets : [];
    const bonusWallets = Array.isArray(summary.bonus_wallets) ? summary.bonus_wallets : [];
    const totalCosts = Array.isArray(summary.total_costs) ? summary.total_costs : [];

    const customWindows: ProviderQuotaWindow[] = [];

    for (const w of normalWallets) {
      const rec = asRecord(w);
      if (rec) {
        const bal = formatMoney(rec.balance, rec.currency);
        customWindows.push({
          label: `Normal Wallet (${bal})`,
          percent: 0,
        });
      }
    }

    for (const w of bonusWallets) {
      const rec = asRecord(w);
      if (rec) {
        const bal = formatMoney(rec.balance, rec.currency);
        customWindows.push({
          label: `Bonus Wallet (${bal})`,
          percent: 0,
        });
      }
    }

    for (const c of totalCosts) {
      const rec = asRecord(c);
      if (rec) {
        const cost = formatMoney(rec.amount ?? rec.value, rec.currency);
        customWindows.push({
          label: `Total Spend (${cost})`,
          percent: 0,
        });
      }
    }

    if (customWindows.length === 0) {
      customWindows.push({
        label: "DSH Account (Active)",
        percent: 0,
      });
    }

    const quota: ProviderQuota = {
      customWindows,
      updatedAt: Date.now(),
    };

    return report(provider, "dsh-account:summary", quota);
  } catch {
    return null;
  }
}
