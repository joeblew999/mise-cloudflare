// Each binding, used once: true when it works, else what it threw. The proof wants every one true.
import { DurableObject, WorkflowEntrypoint } from "cloudflare:workers";

export class Counter extends DurableObject {
  async hit() {
    const n = ((await this.ctx.storage.get("n")) ?? 0) + 1;
    await this.ctx.storage.put("n", n);
    return n;
  }
}

export class Flow extends WorkflowEntrypoint {
  async run(_event, step) {
    return step.do("one step", async () => 1);
  }
}

const works = async (f) => {
  try {
    await f();
    return true;
  } catch (e) {
    return String(e);
  }
};

export default {
  async fetch(_request, env) {
    return Response.json({
      queues: await works(() => env.Q.send({ at: Date.now() })),
      vectorize: await works(() => env.V.describe()),
      ai: typeof env.AI?.run === "function",
      durable_objects: await works(async () => {
        if (!((await env.DO.get(env.DO.idFromName("proof")).hit()) > 0)) throw new Error("no count");
      }),
      workflows: await works(() => env.WF.create()),
      analytics_engine_datasets: await works(() => env.AE.writeDataPoint({ blobs: ["proof"], doubles: [1] })),
      ratelimits: await works(async () => {
        if (typeof (await env.RL.limit({ key: "proof" })).success !== "boolean") throw new Error("no answer");
      }),
      browser: typeof env.BROWSER?.fetch === "function",
      images: typeof env.IMAGES?.input === "function",
    });
  },
};
