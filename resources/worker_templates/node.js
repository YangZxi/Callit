// import { kv, db } from "callit";

/**
{
  "request": {
    "method": "POST",
    "uri": "/test",
    "url": "http://127.0.0.1:3100/test?name=callit",
    "params": {
      "name": "callit"
    },
    "headers": {
      "Content-Type": "application/json",
    },
    "body": {
      "message": "hello",
    },
    "body_str": "..."
  },
  "event": {}
}
*/
// Callit
export default async function handler(ctx) {
    const { request } = ctx;
    // const { headers, body, params } = request;

    // const kvClient = kv.newClient("group1");
    // await kvClient.set("key", "val", 3);
    // const dbClient = db.newClient();
    // const result = await dbClient.exec("select * from users where status = ?", 1);

    return {
        status: 200,
        body: {
            message: "Hello, Callit!",
            request,
        },
        headers: {
            "Content-Type": "application/json"
        }
    }
}
