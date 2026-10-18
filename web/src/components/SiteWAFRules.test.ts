import { describe, expect, it } from "vitest";
import { ruleExceptionRows } from "./SiteWAFRules";

const rule = (id: number) => ({ id, msg: "", file: "", pack: "", disabled: false });

describe("ruleExceptionRows", () => {
  it("lists disabled rules before rules that are still on", () => {
    const rows = ruleExceptionRows([rule(911100), rule(920100), rule(942100)], [942100, 941100]);
    expect(rows.map((r) => r.id)).toEqual([941100, 942100, 911100, 920100]);
  });

  it("keeps a disabled id that matches the search", () => {
    const rows = ruleExceptionRows([rule(941100)], [942100], "942");
    expect(rows.map((r) => r.id)).toEqual([942100, 941100]);
  });
});
