// Callit
function handler(ctx) {
    const { request } = ctx;
    // const { headers, body, params } = request;
    // const { kv } = require("callit");
    // const kvClient = kv.newClient("group1");
    // await kvClient.set("key", "val", 3);
    // const { db } = require("callit");
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

module.exports = handler;
