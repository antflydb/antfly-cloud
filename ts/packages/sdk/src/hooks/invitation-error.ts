/** Structured acceptance failure. Status alone never identifies an email mismatch. */
export class InvitationAcceptanceError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly detail?: string;
  readonly title?: string;

  constructor(status: number, problem: unknown) {
    const body = problem && typeof problem === "object" ? problem : {};
    const stringField = (key: string): string | undefined => {
      const value = (body as Record<string, unknown>)[key];
      return typeof value === "string" && value.trim() ? value : undefined;
    };
    const detail = stringField("detail");
    const code =
      stringField("code") || (detail && /^[A-Z][A-Z_]+$/.test(detail) ? detail : undefined);
    let message: string;
    switch (code) {
      case "MFA_ENROLLMENT_REQUIRED":
        message = "Set up two-factor authentication (MFA) before accepting this invitation.";
        break;
      case "MFA_POLICY_REQUIRED":
        message =
          "Sign in again and complete two-factor authentication (MFA) before accepting this invitation.";
        break;
      default:
        // Preserve human-readable backend reasons, including verified-email and
        // membership conflicts. Unknown machine codes still get useful fallback copy.
        message =
          (detail !== code ? detail : undefined) ||
          stringField("message") ||
          {
            400: "This invitation is invalid or no longer available. Ask the sender for a new invitation.",
            401: "Your session has expired. Sign in again to accept this invitation.",
            403: "Your account cannot accept this invitation. Sign in again or contact the organization administrator.",
            404: "This invitation was not found or has expired. Ask the sender for a new invitation.",
            409: "This invitation conflicts with your existing organization membership. Contact the organization administrator.",
            410: "This invitation has expired. Ask the sender for a new invitation.",
          }[status] ||
          "Unable to accept this invitation. Please try again.";
    }
    super(message);
    this.name = "InvitationAcceptanceError";
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.title = stringField("title");
  }
}
