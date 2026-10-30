// What a fork's build calls itself, in one place: the app's name (branding/,
// scripts/brand.mjs sets VITE_BRAND_NAME) and where its bugs are reported.

export const APP_NAME: string = (import.meta.env?.VITE_BRAND_NAME as string | undefined) || "Shipyard";

export const ISSUES = "https://github.com/tinhthanh/shipyard/issues";
