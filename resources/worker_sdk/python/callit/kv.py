import json
import os
import urllib.error
import urllib.request


def _request(action, payload):
    base_url = (os.environ.get("CALLIT_MAGIC_API_BASE_URL") or "").rstrip("/")
    worker_id = os.environ.get("CALLIT_WORKER_ID") or ""
    request_id = os.environ.get("CALLIT_REQUEST_ID") or ""
    if not base_url or not worker_id or not request_id:
        raise RuntimeError("当前环境缺少 kv 所需上下文")

    request = urllib.request.Request(
        url=f"{base_url}/kv/{action}",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "X-Callit-Worker-Id": worker_id,
            "X-Callit-Request-Id": request_id,
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request) as response:
            raw = response.read().decode("utf-8").strip()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="ignore").strip()
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {}
        message = parsed.get("error") or body or f"kv 请求失败: {error.code}"
        raise RuntimeError(message)
    except urllib.error.URLError as error:
        raise RuntimeError(f"请求 kv 服务失败: {error}") from error


def normalize_namespace(namespace):
    worker_id = (os.environ.get("CALLIT_WORKER_ID") or "").strip()
    if not worker_id:
        raise RuntimeError("当前环境缺少 CALLIT_WORKER_ID")
    if namespace is None:
        return worker_id
    if not isinstance(namespace, str):
        raise TypeError("kv.new_client 的 namespace 必须是 string 或 None")
    normalized_namespace = namespace.strip()
    if not normalized_namespace:
        return worker_id
    return normalized_namespace


class _KVClient:
    def __init__(self, namespace=None):
        self._namespace = normalize_namespace(namespace)

    def set(self, key, value, seconds):
        if not isinstance(value, str):
            raise TypeError("kv client 的 value 必须是 string")
        _request("set", {
            "namespace": self._namespace,
            "key": key,
            "value": value,
            "seconds": seconds,
        })

    def get(self, key):
        response = _request("get", {"namespace": self._namespace, "key": key}) or {}
        return response.get("value")

    def delete(self, key):
        _request("delete", {"namespace": self._namespace, "key": key})

    def increment(self, key, step=1):
        response = _request("increment", {"namespace": self._namespace, "key": key, "step": step}) or {}
        return int(response.get("int_value", 0))

    def expire(self, key, seconds):
        _request("expire", {"namespace": self._namespace, "key": key, "seconds": seconds})

    def ttl(self, key):
        response = _request("ttl", {"namespace": self._namespace, "key": key}) or {}
        return int(response.get("seconds", 0))

    def has(self, key):
        response = _request("has", {"namespace": self._namespace, "key": key}) or {}
        return bool(response.get("exists"))


class _KVFactory:
    def new_client(self, namespace=None):
        return _KVClient(namespace)

    def newClient(self, namespace=None):
        return self.new_client(namespace)


kv = _KVFactory()
