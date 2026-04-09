# Callit
def handler(ctx):
    request = ctx.get("request", {})
    # headers = request.get("headers", {})
    # body = request.get("body", {})
    # params = request.get("params", {})
    # from callit import kv
    # kv_client = kv.new_client("group1")
    # kv_client.set("key", "val", 3)
    # from callit import db
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
