import { describe, it, expect } from "vitest";
import {
  cn,
  formatDate,
  formatRelativeTime,
  slugify,
  isValidSubdomain,
  generateId,
} from "@/lib/utils";

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("handles conditional classes", () => {
    expect(cn("base", false && "hidden", true && "visible")).toBe("base visible");
  });

  it("merges tailwind classes correctly", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
  });
});

describe("formatDate", () => {
  it("formats dates correctly", () => {
    const date = new Date("2024-01-15T10:30:00Z");
    const formatted = formatDate(date);
    expect(formatted).toContain("Jan");
    expect(formatted).toContain("15");
    expect(formatted).toContain("2024");
  });

  it("handles string dates", () => {
    const formatted = formatDate("2024-06-20T15:00:00Z");
    expect(formatted).toContain("Jun");
    expect(formatted).toContain("20");
  });
});

describe("formatRelativeTime", () => {
  it("returns 'just now' for recent times", () => {
    const now = new Date();
    expect(formatRelativeTime(now)).toBe("just now");
  });

  it("returns minutes ago", () => {
    const fiveMinutesAgo = new Date(Date.now() - 5 * 60 * 1000);
    expect(formatRelativeTime(fiveMinutesAgo)).toBe("5m ago");
  });

  it("returns hours ago", () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000);
    expect(formatRelativeTime(threeHoursAgo)).toBe("3h ago");
  });

  it("returns days ago", () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000);
    expect(formatRelativeTime(twoDaysAgo)).toBe("2d ago");
  });
});

describe("slugify", () => {
  it("converts to lowercase", () => {
    expect(slugify("Hello World")).toBe("hello-world");
  });

  it("replaces spaces with hyphens", () => {
    expect(slugify("my app name")).toBe("my-app-name");
  });

  it("removes special characters", () => {
    expect(slugify("Hello! World?")).toBe("hello-world");
  });

  it("removes leading and trailing hyphens", () => {
    expect(slugify("--hello--")).toBe("hello");
  });
});

describe("isValidSubdomain", () => {
  it("returns true for valid subdomains", () => {
    expect(isValidSubdomain("my-app")).toBe(true);
    expect(isValidSubdomain("app123")).toBe(true);
    expect(isValidSubdomain("test")).toBe(true);
  });

  it("returns false for invalid subdomains", () => {
    expect(isValidSubdomain("-invalid")).toBe(false);
    expect(isValidSubdomain("invalid-")).toBe(false);
    expect(isValidSubdomain("UPPERCASE")).toBe(false);
    expect(isValidSubdomain("has spaces")).toBe(false);
    expect(isValidSubdomain("has_underscore")).toBe(false);
  });

  it("returns false for empty strings", () => {
    expect(isValidSubdomain("")).toBe(false);
  });
});

describe("generateId", () => {
  it("generates unique IDs", () => {
    const id1 = generateId();
    const id2 = generateId();
    expect(id1).not.toBe(id2);
  });

  it("generates string IDs", () => {
    expect(typeof generateId()).toBe("string");
  });
});
