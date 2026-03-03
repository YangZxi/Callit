export const siteConfig = {
  name: "Callit",
  description: "Self-hosted personal serverless platform.",
  navItems: [
    { label: "Workers", href: "/workers", mobile: true },
    { label: "Dependencies", href: "/dependencies", mobile: true },
    // { label: "ClipSync", href: "/clipsync" },
    { label: "About", href: "/about" },
  ],
};
export type SiteConfig = typeof siteConfig;
