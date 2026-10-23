export default {
  fetch(_request, env) {
    return new Response(`built from ${env.BUILT_BY || "here"}\n`);
  },
};
