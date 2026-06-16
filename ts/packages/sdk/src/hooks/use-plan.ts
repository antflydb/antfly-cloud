import { useOrganizationSubscription } from "./use-billing";

/**
 * usePlan Hook
 *
 * Provides plan information for the current organization.
 * Compatible with ComponentsAF's usePlan interface.
 *
 * @param orgId - Organization ID to get plan information for
 * @returns Object with plan information
 * - isPro: true if plan is growth or enterprise
 * - isFree: true if plan is free
 * - planType: 'free' | 'growth' | 'enterprise'
 */
export function usePlan(orgId: string | null) {
  const { data: subscription } = useOrganizationSubscription(orgId);

  const planType = subscription?.plan_type || "free";
  const isPro = planType === "growth" || planType === "enterprise";
  const isFree = planType === "free";

  return {
    isPro,
    isFree,
    planType,
  };
}
