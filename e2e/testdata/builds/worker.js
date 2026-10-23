// Which branch built it, and whether its KV and R2 work.
export default {
  async fetch(_request, env) {
    const stamp = new Date().toISOString();
    await env.KV.put("stamp", stamp);
    await env.BUCKET.put("stamp.txt", stamp);
    const kv = (await env.KV.get("stamp")) === stamp;
    const r2 = (await (await env.BUCKET.get("stamp.txt"))?.text()) === stamp;
    return Response.json({ built: env.BUILT_BY || "here", kv, r2 });
  },
};
