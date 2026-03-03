function handler(ctx) {
    const { request } = ctx;
    // const { headers, body, params } = request;
    
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