let BASE_PREFIX = window.__BASE_PREFIX__ || "/admin";
if (BASE_PREFIX === "{{ .base_prefix }}") {
  BASE_PREFIX = "/admin";
  window.__BASE_PREFIX__ = BASE_PREFIX;
}

export { BASE_PREFIX };