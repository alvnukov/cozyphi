// This closure serializes values at the guest boundary. Go owns all effects, including
// the real invocation count, cancellation and reply budget.
((definitions, testing, parallelReads) => {
  const encode = JSON.stringify.bind(JSON);
  const catalog = new Map((definitions || []).map(def => [def.name, def]));
  const pending = new Map();
  const queue = [];
  const tests = [];
  let expectations = [];
  let testActive = false;
  let testFailure = "";
  let nextID = 1;
  let finished = false;
  let started = false;
  let result = null;
  let failure = "";
  const errorMessage = error => [String(error), error?.stack].filter(Boolean).join("\n").slice(0,4096);

  function sorted(value) {
    if (Array.isArray(value)) return value.map(sorted);
    if (value && typeof value === "object")
      return Object.fromEntries(Object.keys(value).sort().map(key => [key, sorted(value[key])]));
    return value;
  }

  function equal(actual, expected) {
    if (encode(sorted(actual)) !== encode(sorted(expected)))
      throw new Error("expected " + encode(expected) + ", received " + encode(actual));
  }

  async function call(name, args = {}) {
    if (!name || typeof name !== "string" || !args || Array.isArray(args) || typeof args !== "object")
      throw new Error("tool calls require a name and an argument object");
    if (testing) {
      const expected = expectations.shift();
      if (!testActive || !expected || expected.name !== name || encode(sorted(expected.args)) !== encode(sorted(args))) {
        testFailure = "unexpected fixture call " + name;
        throw new Error(testFailure);
      }
      if ("error" in expected) throw new Error(expected.error);
      return expected.result;
    }
    if (nextID > 128) throw new Error("cell tool call limit exceeded");
    const id = nextID++;
    const req = {id, name, args};
    if (encode(req).length > 262144) throw new Error("tool request exceeds limit");
    return new Promise((resolve, reject) => {
      pending.set(id, {resolve, reject});
      queue.push(req);
    });
  }

  const api = Object.create(null);
  api.call = call;
  api.list = () => [...catalog.keys()].sort();
  api.describe = query => {
    const names = typeof query === "string" ? [query] : query?.names || [query?.name];
    if (names.length > 8) throw new Error("describe at most eight tools at once");
    const found = names.map(name => catalog.get(name) ?? null);
    return typeof query === "string" || query?.name ? found[0] : found;
  };
  Object.defineProperty(globalThis, "tools", {value: new Proxy(api, { get(t, name) {
    if (typeof name !== "string" || name === "then") return undefined;
    return t[name] ?? (args => call(name, args));
  }})});
  function expect(entry) {
    if (!testActive) throw new Error("declare fixtures inside a running test");
    expectations.push(entry);
  }
  const bindings = {
    start: fn => {
      if (started) throw new Error("cell already started");
      started = true;
      Promise.resolve().then(fn).then(value => {
        result = value ?? null;
        finished = true;
      }, reason => {
        failure = errorMessage(reason);
        finished = true;
      });
    },
    test(name, fn) {
      if (!testing || typeof fn !== "function" || !name) throw new Error("invalid test declaration");
      if (tests.length >= 256) throw new Error("too many tests");
      tests.push([String(name), fn]);
    },
    equal,
    fixture: (name, args, result) => expect({name,args,result}),
    fixtureError: (name, args, error) => expect({name,args,error}),
    async runTests() {
      if (!tests.length) throw new Error("no unit tests registered");
      const names = [];
      for (const [name, fn] of tests) {
        expectations = []; testActive = true; testFailure = "";
        try {
          await fn();
          if (testFailure || expectations.length) throw new Error(testFailure || "unconsumed fixture");
        } catch (err) {
          throw new Error(name + ": " + errorMessage(err));
        } finally { testActive = false; }
        names.push(name);
      }
      return {passed:names.length,tests:names};
    },
    take() {
      if (finished) {
        if (!failure && pending.size) throw new Error("unawaited tool call at end of cell");
        return encode({done:true,result,error:failure});
      }
      const request = queue.shift();
      if (!request) throw new Error("cell is waiting without an outstanding tool call");
      const safeRead = r => r.name === "ls" || r.name === "find" ||
        (r.name === "read" && (!r.args.mode || r.args.mode === "view"));
      if (parallelReads && safeRead(request) && queue.length && safeRead(queue[0])) {
        const batch = [request];
        while (batch.length < 8 && queue.length && safeRead(queue[0])) batch.push(queue.shift());
        return encode({batch});
      }
      return encode(request);
    },
    settle(id, ok, value) {
      const waiter = pending.get(id);
      if (!waiter) throw new Error("unknown host response");
      pending.delete(id);
      if (ok) waiter.resolve(value); else waiter.reject(new Error(value));
    }
  };
  Object.defineProperty(globalThis, "cozyCell", {value:Object.freeze(bindings)});
})
