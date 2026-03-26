# Callit
def handler(ctx):
    request = ctx.get("request", {})
    # headers = request.get("headers", {})
    # body = request.get("body", {})
    # params = request.get("params", {})

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