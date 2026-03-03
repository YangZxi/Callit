import json
import importlib.util
import sys  

raw = sys.stdin.read()
ctx = json.loads(raw) if raw else {}

spec = importlib.util.spec_from_file_location("callit_main", "main.py")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)

handler = getattr(mod, "handler", None)
if not callable(handler):
	raise RuntimeError("main.py 必须定义 handler(ctx)")

out = handler(ctx)
print(json.dumps(out, ensure_ascii=False))
