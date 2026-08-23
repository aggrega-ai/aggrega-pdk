// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Reference policy plugin that drives a pre-baked OPA-compiled
//! policy module with the full OPA evaluation semantics and builtin surface.
//!
//! The policy is authored in Rego, compiled once with OPA's own toolchain
//! (`opa build -t wasm`) into a WebAssembly module, and committed alongside
//! this source as `policy.wasm`. The adapter embeds that module and, on every
//! build, instantiates it inside this guest with an embedded WebAssembly
//! interpreter, satisfies the OPA module's own imports (its linear memory and
//! its builtin callbacks) with in-guest host functions, runs the OPA
//! evaluation sequence, and maps the OPA decision back to the verdict shape the
//! policy socket expects. The OPA Wasm ABI is absorbed entirely here: the
//! compiler host sees only the standard `describe()` / `verdict_ir()` surface and
//! carries no OPA-specific logic.
//!
//! Baking the policy ahead of time keeps evaluation self-contained: no OPA
//! process runs, nothing is fetched, and the committed module is a pinned,
//! reproducible artifact.
//!
//! Authoring convention. The baked policy declares its decision rules in a Rego
//! package and exposes up to three collections of message strings, each
//! compiled as an OPA entrypoint:
//!
//!   - `deny`  — each message stops the build.
//!   - `warn`  — each message flags the build but lets it proceed.
//!   - `audit` — each message is recorded without affecting the outcome.
//!
//! Those three Rego rule names are Rego's own; the adapter maps them onto the
//! socket's three verdict severities — `deny` lands as `halt`, `warn` as `warn`,
//! and `audit` as `info`.
//!
//! The adapter reads the module's entrypoint table and maps each entrypoint's
//! trailing name to a verdict severity, so a policy chooses its severities by
//! which of `deny` / `warn` / `audit` it compiles as entrypoints.
//!
//! Input mapping. The whole policy payload is the OPA `input` document: a
//! policy reaches the IR graph as `input.graph` and the routing paths as
//! `input.precalculated_paths`. The attachment's `settings` object becomes the
//! OPA `data` document, so a policy reads an operator-supplied tunable as
//! `data.<key>`.
//!
//! Loud failure. The baked module declares which external builtins it needs. A
//! policy compiled to need a host-provided builtin cannot be satisfied in-guest;
//! the adapter refuses such a module up front, and every builtin callback is an
//! error stub as a backstop, so a policy that reaches for an unprovided builtin
//! fails the build loudly rather than passing silently. "The adapter cannot
//! evaluate this policy" never reads as "the policy allows the build".

use aggrega_pdk_rust::{
    SEVERITY_HALT, SEVERITY_INFO, SEVERITY_WARN,
    encode_descriptor, PluginDescriptor, PluginDescriptorMeta, PluginExport, PolicyOutput,
    Diagnostic, API_VERSION,
};
use anyhow::{anyhow, bail, Context};
use extism_pdk::{plugin_fn, FnResult};
use wasmi::{Caller, Engine, Linker, Memory, MemoryType, Module, Store};

/// The baked OPA-compiled policy module, embedded at build time. Regenerate it
/// from policy.rego with the OPA CLI as documented in the README.
static POLICY_WASM: &[u8] = include_bytes!("../policy.wasm");

/// The static `describe()` self-report: a v1alpha1 `policy` plugin
/// evaluating the ir stage, whose `verdict_ir` export the host validates before
/// dispatch.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description:
            "Evaluate an OPA-compiled Rego policy against the build graph, driving the baked policy module in-guest."
                .to_string(),
        exports: vec![PluginExport {
            name: "verdict_ir".to_string(),
            ..Default::default()
        }],
        metadata: Some(Box::new(PluginDescriptorMeta {
            author: "aggrega".to_string(),
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    }
}

/// The host calls `describe()` once at load to read the plugin's static
/// self-report before dispatching `verdict_ir`.
#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(encode_descriptor(&descriptor()))
}

/// The host calls `verdict_ir(input)` with the policy payload as a JSON string
/// and reads back the fired verdicts. A drive or evaluation failure propagates
/// as an error, which the host surfaces as a build fault naming this call.
#[plugin_fn]
pub fn verdict_ir(input: String) -> FnResult<String> {
    let results = evaluate(&input)?;
    Ok(serde_json::to_string(&PolicyOutput {
        results,
        ..Default::default()
    })?)
}

/// ExternalBuiltinInvoked reports a baked policy reaching for a host-provided
/// OPA builtin the adapter does not implement. It surfaces as a Wasm trap that
/// propagates out of `verdict_ir` as a build fault, never as an allow verdict.
#[derive(Debug)]
struct ExternalBuiltinInvoked {
    id: i32,
}

impl core::fmt::Display for ExternalBuiltinInvoked {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(
            f,
            "policy module invoked external builtin id {} which the adapter does not provide",
            self.id
        )
    }
}

impl wasmi::core::HostError for ExternalBuiltinInvoked {}

/// OpaAbort carries the message the OPA module passes to its abort callback.
#[derive(Debug)]
struct OpaAbort(String);

impl core::fmt::Display for OpaAbort {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(f, "policy module aborted: {}", self.0)
    }
}

impl wasmi::core::HostError for OpaAbort {}

/// evaluate parses the payload, wires it into the baked OPA module, drives the
/// evaluation entrypoints, and collects the fired verdicts. It returns an error
/// for any genuine drive or evaluation failure so the caller surfaces a build
/// fault; an entrypoint whose decision set is empty is not a failure and simply
/// contributes no verdict.
fn evaluate(raw: &str) -> anyhow::Result<Vec<Diagnostic>> {
    let payload: serde_json::Value =
        serde_json::from_str(raw).context("policy payload is not valid JSON")?;

    // The attachment's settings object becomes the OPA data document; an absent
    // or null settings object is the empty data document.
    let data_json = match payload.get("settings") {
        Some(s) if !s.is_null() => s.to_string(),
        _ => "{}".to_string(),
    };

    drive(raw, &data_json)
}

/// read_cstr reads a NUL-terminated string out of the module's linear memory.
fn read_cstr(mem: &[u8], addr: usize) -> String {
    let end = mem[addr..]
        .iter()
        .position(|&b| b == 0)
        .map(|p| addr + p)
        .unwrap_or(mem.len());
    String::from_utf8_lossy(&mem[addr..end]).into_owned()
}

/// leaf_severity maps an OPA entrypoint path to a verdict severity by its
/// trailing segment. An entrypoint whose name is not one of the three the
/// adapter recognizes is a policy authored against a contract this adapter does
/// not understand and fails loudly.
///
/// The names on the left are Rego decision-rule names, which follow Rego's own
/// naming for a decision rule; the values on the right are the socket's verdict
/// severities. This adapter is where one becomes the other, which is why the two
/// sides do not read alike.
fn leaf_severity(entrypoint: &str) -> anyhow::Result<&'static str> {
    let leaf = entrypoint.rsplit('/').next().unwrap_or(entrypoint);
    match leaf {
        "deny" => Ok(SEVERITY_HALT),
        "warn" => Ok(SEVERITY_WARN),
        "audit" => Ok(SEVERITY_INFO),
        other => Err(anyhow!(
            "policy entrypoint {entrypoint:?} names no decision rule this adapter evaluates: {other:?}; expected deny, warn, or audit"
        )),
    }
}

/// drive instantiates the baked OPA module inside this guest, satisfies its
/// imports, and runs the OPA evaluation sequence over every entrypoint the
/// module declares, returning the fired verdicts.
fn drive(payload: &str, data_json: &str) -> anyhow::Result<Vec<Diagnostic>> {
    let engine = Engine::default();
    let module = Module::new(&engine, POLICY_WASM)
        .context("baked policy module is not a valid WebAssembly module")?;
    let mut store: Store<()> = Store::new(&engine, ());
    let mut linker: Linker<()> = Linker::new(&engine);

    // The OPA module imports env.memory; provide it. The module grows this
    // memory itself through its own allocator as evaluation proceeds.
    let memory = Memory::new(&mut store, MemoryType::new(2, Some(65536))?)?;
    linker.define("env", "memory", memory)?;

    // opa_abort(addr) reports an unrecoverable module-side error at a message
    // pointer; surface it as a trap rather than letting evaluation continue.
    let abort_mem = memory;
    linker.func_wrap(
        "env",
        "opa_abort",
        move |caller: Caller<()>, addr: i32| -> Result<(), wasmi::Error> {
            let msg = read_cstr(abort_mem.data(&caller), addr as usize);
            Err(wasmi::Error::host(OpaAbort(msg)))
        },
    )?;

    // The OPA builtin callbacks are error stubs. The baked reference policy
    // needs none (its builtins compile into the module); a policy that reaches
    // for a host-provided builtin traps here rather than receiving a wrong or
    // empty answer.
    linker.func_wrap(
        "env",
        "opa_builtin0",
        |_: Caller<()>, id: i32, _ctx: i32| -> Result<i32, wasmi::Error> {
            Err(wasmi::Error::host(ExternalBuiltinInvoked { id }))
        },
    )?;
    linker.func_wrap(
        "env",
        "opa_builtin1",
        |_: Caller<()>, id: i32, _ctx: i32, _a: i32| -> Result<i32, wasmi::Error> {
            Err(wasmi::Error::host(ExternalBuiltinInvoked { id }))
        },
    )?;
    linker.func_wrap(
        "env",
        "opa_builtin2",
        |_: Caller<()>, id: i32, _ctx: i32, _a: i32, _b: i32| -> Result<i32, wasmi::Error> {
            Err(wasmi::Error::host(ExternalBuiltinInvoked { id }))
        },
    )?;
    linker.func_wrap(
        "env",
        "opa_builtin3",
        |_: Caller<()>, id: i32, _ctx: i32, _a: i32, _b: i32, _c: i32| -> Result<i32, wasmi::Error> {
            Err(wasmi::Error::host(ExternalBuiltinInvoked { id }))
        },
    )?;
    linker.func_wrap(
        "env",
        "opa_builtin4",
        |_: Caller<()>,
         id: i32,
         _ctx: i32,
         _a: i32,
         _b: i32,
         _c: i32,
         _d: i32|
         -> Result<i32, wasmi::Error> {
            Err(wasmi::Error::host(ExternalBuiltinInvoked { id }))
        },
    )?;

    let instance = linker
        .instantiate(&mut store, &module)?
        .start(&mut store)?;

    let opa_malloc = instance.get_typed_func::<i32, i32>(&store, "opa_malloc")?;
    let opa_json_parse = instance.get_typed_func::<(i32, i32), i32>(&store, "opa_json_parse")?;
    let ctx_new = instance.get_typed_func::<(), i32>(&store, "opa_eval_ctx_new")?;
    let set_input = instance.get_typed_func::<(i32, i32), ()>(&store, "opa_eval_ctx_set_input")?;
    let set_data = instance.get_typed_func::<(i32, i32), ()>(&store, "opa_eval_ctx_set_data")?;
    let set_entrypoint =
        instance.get_typed_func::<(i32, i32), ()>(&store, "opa_eval_ctx_set_entrypoint")?;
    let eval = instance.get_typed_func::<i32, i32>(&store, "eval")?;
    let get_result = instance.get_typed_func::<i32, i32>(&store, "opa_eval_ctx_get_result")?;
    let opa_json_dump = instance.get_typed_func::<i32, i32>(&store, "opa_json_dump")?;
    let builtins = instance.get_typed_func::<(), i32>(&store, "builtins")?;
    let entrypoints = instance.get_typed_func::<(), i32>(&store, "entrypoints")?;

    // dump_json reads an OPA value address back as a JSON string.
    let dump_json = |store: &mut Store<()>, value_addr: i32| -> anyhow::Result<String> {
        let str_addr = opa_json_dump.call(&mut *store, value_addr)?;
        Ok(read_cstr(memory.data(&*store), str_addr as usize))
    };

    // The module lists the external builtins it needs. This adapter satisfies
    // none in-guest, so a non-empty list is a policy the adapter cannot honour;
    // refuse it up front with a specific message rather than trapping mid-eval.
    let builtins_addr = builtins.call(&mut store, ())?;
    let builtins_json = dump_json(&mut store, builtins_addr)?;
    if builtins_json.trim() != "{}" {
        bail!(
            "baked policy module requires host-provided builtins {builtins_json}; \
             the adapter provides none. Recompile the policy without those builtins."
        );
    }

    // The module maps each entrypoint name to its numeric id.
    let entrypoints_addr = entrypoints.call(&mut store, ())?;
    let entrypoints_json = dump_json(&mut store, entrypoints_addr)?;
    let entrypoint_map: serde_json::Map<String, serde_json::Value> =
        serde_json::from_str(&entrypoints_json)
            .context("policy module entrypoint table is not a JSON object")?;

    // Evaluate entrypoints in id order for a deterministic verdict sequence.
    let mut entrypoints_by_id: Vec<(i32, String)> = entrypoint_map
        .into_iter()
        .map(|(name, id)| {
            let id = id
                .as_i64()
                .ok_or_else(|| anyhow!("entrypoint {name:?} has a non-integer id {id}"))?;
            Ok((id as i32, name))
        })
        .collect::<anyhow::Result<Vec<_>>>()?;
    entrypoints_by_id.sort_by_key(|(id, _)| *id);

    // Write the input and data documents into the module once and parse each to
    // an OPA value the eval contexts reuse.
    let input_value = parse_json(&mut store, &opa_malloc, &opa_json_parse, memory, payload)?;
    let data_value = parse_json(&mut store, &opa_malloc, &opa_json_parse, memory, data_json)?;

    let mut results = Vec::new();
    for (id, name) in entrypoints_by_id {
        let severity = leaf_severity(&name)?;

        let ctx = ctx_new.call(&mut store, ())?;
        set_input.call(&mut store, (ctx, input_value))?;
        set_data.call(&mut store, (ctx, data_value))?;
        set_entrypoint.call(&mut store, (ctx, id))?;
        let rc = eval.call(&mut store, ctx)?;
        if rc != 0 {
            bail!("policy entrypoint {name:?} evaluation returned status {rc}");
        }
        let result_addr = get_result.call(&mut store, ctx)?;
        let result_json = dump_json(&mut store, result_addr)?;
        collect(&result_json, severity, &name, &mut results)?;
    }

    Ok(results)
}

/// parse_json copies a JSON string into the module's memory and parses it to an
/// OPA value, returning that value's address.
fn parse_json(
    store: &mut Store<()>,
    opa_malloc: &wasmi::TypedFunc<i32, i32>,
    opa_json_parse: &wasmi::TypedFunc<(i32, i32), i32>,
    memory: Memory,
    json: &str,
) -> anyhow::Result<i32> {
    let bytes = json.as_bytes();
    let addr = opa_malloc.call(&mut *store, bytes.len() as i32)?;
    memory.data_mut(&mut *store)[addr as usize..addr as usize + bytes.len()]
        .copy_from_slice(bytes);
    Ok(opa_json_parse.call(&mut *store, (addr, bytes.len() as i32))?)
}

/// collect reads one entrypoint's evaluation result and appends a verdict of
/// the given severity for each message it holds. An OPA entrypoint result is an
/// array of `{"result": <rule-value>}`; the rule value for a decision set is an
/// array of message strings. An empty result contributes no verdict; a result
/// whose messages are not strings is a malformed policy and fails loudly.
fn collect(
    result_json: &str,
    severity: &str,
    entrypoint: &str,
    out: &mut Vec<Diagnostic>,
) -> anyhow::Result<()> {
    let value: serde_json::Value = serde_json::from_str(result_json)
        .with_context(|| format!("policy entrypoint {entrypoint:?} result is not valid JSON"))?;
    let items = value.as_array().ok_or_else(|| {
        anyhow!("policy entrypoint {entrypoint:?} result is not a JSON array: {result_json}")
    })?;
    for item in items {
        let Some(rule_value) = item.get("result") else {
            continue;
        };
        let messages = rule_value.as_array().ok_or_else(|| {
            anyhow!(
                "policy entrypoint {entrypoint:?} decision is not an array of message strings: {rule_value}"
            )
        })?;
        for message in messages {
            let text = message.as_str().ok_or_else(|| {
                anyhow!("policy entrypoint {entrypoint:?} emitted a non-string message: {message}")
            })?;
            out.push(Diagnostic {
                severity: severity.to_string(),
                message: text.to_string(),
                ..Default::default()
            });
        }
    }
    Ok(())
}
