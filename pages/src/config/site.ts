export const siteConfig = {
  name: "Callit",
  description: "Self-hosted personal serverless platform.",
  navItems: [
    { label: "Dashboard", href: `${window.__BASE_PREFIX__}/dashboard`, mobile: true },
    { label: "Workers", href: `${window.__BASE_PREFIX__}/workers`, mobile: true },
    { label: "Dependencies", href: `${window.__BASE_PREFIX__}/dependencies`, mobile: true },
    { label: "Config", href: `${window.__BASE_PREFIX__}/config`, mobile: true },
    // { label: "ClipSync", href: `${window.__BASE_PREFIX__}/clipsync` },
    { label: "About", href: `${window.__BASE_PREFIX__}/about` },
  ],
};
export type SiteConfig = typeof siteConfig;
