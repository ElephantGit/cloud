import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { PluginLogo } from "../contracts";
import { act } from "react";
import { PluginLogoMark } from "./plugin-logo";

const UNIVERSAL: PluginLogo = {
  variant: "universal",
  url: "ora-plugin://localhost/logo/official/acme.hub/universal.svg",
};

const THEMED: PluginLogo = {
  variant: "themed",
  light: "ora-plugin://localhost/logo/official/acme.hub/light.svg",
  dark: "ora-plugin://localhost/logo/official/acme.hub/dark.png",
};

/** Marks the page light or dark the way frontend-v2 does, which is what the mark reads. */
function setTheme(theme: "light" | "dark" | "system") {
  act(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
  });
}

afterEach(() => {
  setTheme("system");
});

describe("PluginLogoMark", () => {
  it("draws a universal icon identically under both themes", () => {
    setTheme("light");
    const { rerender } = render(<PluginLogoMark logo={UNIVERSAL} />);
    const inLight = screen.getByRole("presentation", { hidden: true });
    expect(inLight).toHaveAttribute("src", UNIVERSAL.url);

    // The host never recolours or inverts an icon, so a plugin shipping one file gets exactly
    // those pixels on a dark background too.
    setTheme("dark");
    rerender(<PluginLogoMark logo={UNIVERSAL} />);
    expect(screen.getByRole("presentation", { hidden: true })).toHaveAttribute(
      "src",
      UNIVERSAL.url,
    );
  });

  it("draws the half of a theme pair matching the active theme", () => {
    setTheme("light");
    const { rerender } = render(<PluginLogoMark logo={THEMED} />);
    expect(screen.getByRole("presentation", { hidden: true })).toHaveAttribute(
      "src",
      THEMED.variant === "themed" ? THEMED.light : "",
    );

    setTheme("dark");
    rerender(<PluginLogoMark logo={THEMED} />);
    // The two halves may ship in different formats; only the URL decides what is drawn.
    expect(screen.getByRole("presentation", { hidden: true })).toHaveAttribute(
      "src",
      THEMED.variant === "themed" ? THEMED.dark : "",
    );
  });

  it("falls back to a generic mark when a plugin ships no icon", () => {
    const { container } = render(<PluginLogoMark logo={null} />);

    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).not.toBeNull();
  });
});
