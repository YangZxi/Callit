let BASE_PREFIX = window.__BASE_PREFIX__ || "/admin";
if (BASE_PREFIX === "{{ .base_prefix }}") {
  BASE_PREFIX = "/admin";
}
// replace icon href
const iconLink = document.querySelector('link[rel="icon"]') as HTMLLinkElement | null;
if (iconLink) {
  iconLink.href = window.__BASE_PREFIX__ + '/static/favicon.ico';
}
if (!BASE_PREFIX.startsWith("/")) {
  BASE_PREFIX = `/${BASE_PREFIX}`;
}
if (BASE_PREFIX.length > 1 && BASE_PREFIX.endsWith("/")) {
  BASE_PREFIX = BASE_PREFIX.slice(0, -1);
}
window.__BASE_PREFIX__ = BASE_PREFIX;

export { BASE_PREFIX };
