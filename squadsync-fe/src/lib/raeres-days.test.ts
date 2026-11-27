// Run with `bun test` (also part of `make test`).
import assert from "node:assert/strict";
import { describe, test } from "node:test";
import {
  addDays,
  budapestToday,
  daysBetween,
  daysInMonth,
  firstOfMonth,
  nextMonth,
  weekdayMondayFirst,
} from "./raeres-days";

describe("budapestToday", () => {
  test("is already the next day in Budapest late in the evening UTC", () => {
    assert.equal(budapestToday(new Date("2026-03-02T23:30:00Z")), "2026-03-03");
    assert.equal(budapestToday(new Date("2026-03-02T09:00:00Z")), "2026-03-02");
  });

  test("follows summer time", () => {
    assert.equal(budapestToday(new Date("2026-07-01T22:30:00Z")), "2026-07-02");
    assert.equal(budapestToday(new Date("2026-12-01T22:30:00Z")), "2026-12-01");
  });
});

describe("day arithmetic", () => {
  test("adds days across months, years and the clock change", () => {
    assert.equal(addDays("2026-03-02", 365), "2027-03-02");
    assert.equal(addDays("2026-12-31", 1), "2027-01-01");
    assert.equal(addDays("2026-10-24", 2), "2026-10-26");
    assert.equal(addDays("2026-03-01", -1), "2026-02-28");
  });

  test("knows months", () => {
    assert.equal(daysInMonth("2026-02-10"), 28);
    assert.equal(daysInMonth("2028-02-10"), 29);
    assert.equal(firstOfMonth("2026-03-17"), "2026-03-01");
    assert.equal(nextMonth("2026-12-17"), "2027-01-01");
  });

  test("starts the week on Monday", () => {
    assert.equal(weekdayMondayFirst("2026-03-02"), 0); // a Monday
    assert.equal(weekdayMondayFirst("2026-03-08"), 6); // a Sunday
  });

  test("lists days both ends included", () => {
    assert.deepEqual(daysBetween("2026-03-01", "2026-03-03"), [
      "2026-03-01",
      "2026-03-02",
      "2026-03-03",
    ]);
    assert.deepEqual(daysBetween("2026-03-03", "2026-03-01"), []);
  });
});
