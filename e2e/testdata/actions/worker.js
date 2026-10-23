// Which Worker this is and whether its KV works; the proof adds pr: true on its pull request's branch.
export default {
  async fetch(_request, env) {
    const stamp = new Date().toISOString();
    await env.KV.put("stamp", stamp);
    const kv = (await env.KV.get("stamp")) === stamp;
    return Response.json({ worker: "cloudflare-zero-actions", kv });
  },
};
