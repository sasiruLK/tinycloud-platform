import type { ApiErrorResponse } from "@/types/api";

export class ApiError extends Error {
  public readonly status: number;
  public readonly errorCode: string;
  public readonly requestId: string;

  constructor(response: ApiErrorResponse) {
    super(response.message);
    this.name = "ApiError";
    this.status = response.status;
    this.errorCode = response.error;
    this.requestId = response.requestId;
  }

  getFriendlyMessage(): string {
    switch (this.errorCode) {
      case "unauthorized":
        return "Authentication required. Please sign in.";
      case "not_found":
        return this.message;
      case "bad_request":
        return this.message;
      case "internal_error":
        return "Something went wrong on our end. Please try again later.";
      default:
        return this.message || "An unexpected error occurred.";
    }
  }
}
