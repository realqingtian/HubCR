import { describe, expect, it } from "vitest";
import { healthResponseSchema } from "./schemas";

describe("healthResponseSchema", () => {
  it("accepts the control-plane health response", () => {
    expect(healthResponseSchema.parse({ status: "ok" })).toEqual({ status: "ok" });
  });

  it("rejects an unknown health state", () => {
    expect(() => healthResponseSchema.parse({ status: "unknown" })).toThrow();
  });
});
