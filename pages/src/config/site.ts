export const siteConfig = {
  name: "Callit",
  description: "Self-hosted personal serverless platform.",
  navItems: [
    { labelKey: "nav.dashboard", href: `${window.__BASE_PREFIX__}/dashboard`, mobile: true },
    { labelKey: "nav.workers", href: `${window.__BASE_PREFIX__}/workers`, mobile: true },
    { labelKey: "nav.dependencies", href: `${window.__BASE_PREFIX__}/dependencies`, mobile: true },
    {
      labelKey: "nav.storage", href: `${window.__BASE_PREFIX__}/storage/kv`, mobile: true,
      children: [
        { labelKey: "KV", href: `${window.__BASE_PREFIX__}/storage/kv`, mobile: true },
        { labelKey: "DB", href: `${window.__BASE_PREFIX__}/storage/db`, mobile: true },
      ],
    },
    { labelKey: "nav.config", href: `${window.__BASE_PREFIX__}/config`, mobile: true },
    // { label: "ClipSync", href: `${window.__BASE_PREFIX__}/clipsync` },
    { labelKey: "nav.about", href: `${window.__BASE_PREFIX__}/about` },
  ],
};
export type SiteConfig = typeof siteConfig;
