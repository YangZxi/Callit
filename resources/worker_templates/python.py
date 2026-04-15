# from callit import kv, db

# {
#   "request": {
#     "method": "POST",
#     "uri": "/test",
#     "url": "http://127.0.0.1:3100/test?name=callit",
#     "params": {
#       "name": "callit"
#     },
#     "headers": {
#       "Content-Type": "application/json",
#     },
#     "body": {
#       "message": "hello",
#     },
#     "body_str": "..."
#   },
#   "event": {}
# }
# Callit
def handler(ctx):
    request = ctx.get("request", {})
    # headers = request.get("headers", {})
    # body = request.get("body", {})
    # params = request.get("params", {})

    # kv_client = kv.new_client("group1")
    # kv_client.set("key", "val", 3)
    # db_client = db.new_client()
    # result = db_client.exec("select * from users where status = ?", 1)

    return {
        "status": 200,
        "body": {
            "message": "Hello, Callit!",
            "request": request
        },
        "headers": {
            "Content-Type": "application/json"
        }
    }
