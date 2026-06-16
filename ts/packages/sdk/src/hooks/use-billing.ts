import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { client } from "../client";
import type { components } from "../types";

// Export types from OpenAPI schema
export type UsageSummary = components["schemas"]["UsageSummary"];
export type Subscription = components["schemas"]["Subscription"];
export type BillingState = components["schemas"]["BillingState"];
export type BillingCreditLedgerList = components["schemas"]["BillingCreditLedgerList"];
export type BillingSpendRecordList = components["schemas"]["BillingSpendRecordList"];
export type BillingSpendLimit = components["schemas"]["BillingSpendLimit"];

export type BillingPortalSession = {
  url: string;
  type: "portal" | "checkout" | "setup";
};

export type BillingSessionIntent = "setup_payment_method" | "start_subscription" | "manage_billing";

/**
 * Fetches organization usage metrics for the current billing period
 * @param orgId - Organization ID
 */
export function useOrganizationUsage(orgId: string | null | undefined) {
  return useQuery({
    queryKey: ["organization", orgId, "usage"],
    queryFn: async (): Promise<UsageSummary> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/usage", {
        params: { path: { org_id: orgId } },
      });

      if (error) {
        // Handle error with proper type checking for detail property
        const errorMessage =
          typeof error === "object" && error !== null && "detail" in error
            ? String(error.detail)
            : "Failed to fetch usage data";
        throw new Error(errorMessage);
      }

      return data;
    },
    enabled: !!orgId,
  });
}

/**
 * Fetches normalized billing state for dashboard surfaces.
 */
export function useBillingState(orgId: string | null | undefined) {
  return useQuery({
    queryKey: ["organization", orgId, "billing-state"],
    queryFn: async (): Promise<BillingState> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/billing/state", {
        params: { path: { org_id: orgId } },
      });

      if (error) {
        const errorMessage =
          typeof error === "object" && error !== null && "detail" in error
            ? String(error.detail)
            : "Failed to fetch billing state";
        throw new Error(errorMessage);
      }

      return data;
    },
    enabled: !!orgId,
  });
}

export function useCreditLedger(
  orgId: string | null | undefined,
  options?: { productFamily?: "ai" | "cloud"; limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organization", orgId, "credit-ledger", options],
    queryFn: async (): Promise<BillingCreditLedgerList> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/billing/credit-ledger", {
        params: {
          path: { org_id: orgId },
          query: {
            product_family: options?.productFamily,
            limit: options?.limit,
            offset: options?.offset,
          },
        },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch credit ledger");
      }

      return data;
    },
    enabled: !!orgId,
  });
}

export function useSpendRecords(
  orgId: string | null | undefined,
  options?: { productFamily?: "ai" | "cloud"; limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organization", orgId, "spend-records", options],
    queryFn: async (): Promise<BillingSpendRecordList> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/billing/spend-records", {
        params: {
          path: { org_id: orgId },
          query: {
            product_family: options?.productFamily,
            limit: options?.limit,
            offset: options?.offset,
          },
        },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch spend records");
      }

      return data;
    },
    enabled: !!orgId,
  });
}

export function useUpsertSpendLimit(orgId: string | null | undefined) {
  return useMutation({
    mutationFn: async (body: {
      product_family: "ai" | "cloud";
      period: "monthly";
      limit_cents: number;
      alert_threshold_cents?: number;
      enforcement_enabled?: boolean;
    }): Promise<BillingSpendLimit> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.PUT("/organizations/{org_id}/billing/spend-limits", {
        params: { path: { org_id: orgId } },
        body,
      });

      if (error) {
        throw new Error(error.detail || "Failed to save spend limit");
      }

      return data;
    },
  });
}

/**
 * Fetches organization subscription details
 * @param orgId - Organization ID
 */
export function useOrganizationSubscription(orgId: string | null | undefined) {
  return useQuery({
    queryKey: ["organization", orgId, "subscription"],
    queryFn: async (): Promise<Subscription> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/subscription", {
        params: { path: { org_id: orgId } },
      });

      if (error) {
        // Handle error with proper type checking for detail property
        const errorMessage =
          typeof error === "object" && error !== null && "detail" in error
            ? String(error.detail)
            : "Failed to fetch subscription";
        throw new Error(errorMessage);
      }

      return data;
    },
    enabled: !!orgId,
  });
}

/**
 * Creates a Stripe billing portal session and redirects the user
 * @param orgId - Organization ID
 */
export function useCreateBillingPortalSession(
  orgId: string | null | undefined,
  options?: {
    intent?: BillingSessionIntent;
    returnUrl?: string;
    plan?: "growth" | "enterprise";
  }
) {
  return useMutation({
    mutationFn: async (): Promise<BillingPortalSession> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.POST("/organizations/{org_id}/subscription/portal", {
        params: { path: { org_id: orgId } },
        body: {
          return_url: options?.returnUrl ?? window.location.href,
          intent: options?.intent,
          plan: options?.plan,
        },
      });

      if (error) {
        // Handle error with proper type checking for detail property
        const errorMessage =
          typeof error === "object" && error !== null && "detail" in error
            ? String(error.detail)
            : "Failed to create portal session";
        throw new Error(errorMessage);
      }

      return data;
    },
    onSuccess: (data) => {
      // Show appropriate message based on session type
      const message =
        data.type === "setup"
          ? "Redirecting to secure payment setup..."
          : data.type === "checkout"
            ? "Redirecting to upgrade checkout..."
            : "Redirecting to billing portal...";

      toast.info(message);

      // Redirect to Stripe (checkout or portal)
      window.location.href = data.url;
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Failed to open billing portal");
    },
  });
}
