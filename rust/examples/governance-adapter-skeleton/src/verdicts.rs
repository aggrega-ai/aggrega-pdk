// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! The ir-verdict and render-verdict roles, on one `validate` export.
//!
//! One export serves both because the payload identifies itself by shape: the
//! render-verdict payload carries the rendered output files, the ir-verdict
//! payload carries the build graph. Reading the shape is what lets this guest
//! serve both without declaring which role it is attached at — and the role, not
//! the guest, is what decides which payload arrives.
//!
//! # What arrives
//!
//! At the ir-verdict role: the build graph, and the routing paths enumerated
//! alongside it so a policy can reason about what reaches what without walking
//! the graph itself.
//!
//! At the render-verdict role: the rendered output files, each with its output
//! path, its rendered text, the format it was rendered in, and — where the host
//! parsed the content before handing it over — a structured view of that content,
//! so a policy can reason about a document it does not read natively. A file the
//! host did not parse carries no structured view and is read as text.
//!
//! # What may be emitted
//!
//! Verdicts, and nothing else. A verdict role reports an outcome; the build reads
//! it as an outcome and never applies it as a change. An empty result means no
//! verdict fired, which is a policy that allowed the build.
//!
//! # Decision collections are not wire severities
//!
//! `deny`, `warn` and `audit` are decision collections — a convention many policy
//! languages already group decisions under, adopted here so a policy author can
//! reuse what they know. An adapter is free to pick other names, or to read them
//! from the attachment's settings so an operator can choose.
//!
//! The severities the wire carries are a separate, closed set of three: `halt`
//! stops the build and names what to fix, `warn` surfaces alongside a build that
//! still completes, and `info` records the observation and leaves the build's
//! outcome unchanged. [`COLLECTIONS`] is the mapping between the two, and keeping
//! them apart is what lets a policy author rename a decision collection without
//! changing what the build does with it.

use aggrega_abi::{PolicySource, RenderPolicyInput};
use aggrega_pdk_rust::{
    Diagnostic, PolicyInput, PolicyOutput, SEVERITY_HALT, SEVERITY_INFO, SEVERITY_WARN,
};

use crate::evaluator::{self, Program};

/// The decisions the verdict roles ask the engine for.
pub const DENY_DECISION: &str = "deny";
pub const WARN_DECISION: &str = "warn";
pub const AUDIT_DECISION: &str = "audit";

/// Each decision collection paired with the severity this adapter reports it as,
/// in the order the results are emitted.
pub const COLLECTIONS: [(&str, &str); 3] = [
    (DENY_DECISION, SEVERITY_HALT),
    (WARN_DECISION, SEVERITY_WARN),
    (AUDIT_DECISION, SEVERITY_INFO),
];

/// What the two verdict payloads have in common once the facet is known: the role
/// that delivered it and the programs and settings it carried.
struct Facet {
    role: &'static str,
    sources: Vec<PolicySource>,
    settings: Option<Box<serde_json::value::RawValue>>,
}

/// Reads the payload, asks the engine for its verdict decisions, and returns the
/// verdicts the build reports.
pub fn verdicts(raw: &str) -> anyhow::Result<PolicyOutput> {
    let facet = facet(raw)?;
    let programs: Vec<Program<'_>> = facet
        .sources
        .iter()
        .map(|s| Program {
            path: &s.path,
            text: &s.content,
        })
        .collect();
    let settings = facet.settings.as_ref().map(|s| s.get());

    let mut results = Vec::new();
    for (decision, severity) in COLLECTIONS {
        let reached = evaluator::evaluate(&programs, settings, raw, decision).map_err(|err| {
            anyhow::anyhow!("evaluating the {} policy: {err}", facet.role)
        })?;
        for value in &reached {
            let message: String = serde_json::from_str(value.get()).map_err(|err| {
                anyhow::anyhow!(
                    "a decision collected under {decision:?} is not a message: {err}. \
                     A verdict decision is the text the build reports."
                )
            })?;
            results.push(Diagnostic {
                severity: severity.to_string(),
                message,
                ..Default::default()
            });
        }
    }

    Ok(PolicyOutput {
        results,
        ..Default::default()
    })
}

/// Reads the payload and reports which verdict role delivered it.
///
/// Which facet arrived is read from the payload rather than assumed, so a payload
/// this guest cannot read is refused instead of being evaluated as the other facet
/// and quietly finding nothing.
fn facet(raw: &str) -> anyhow::Result<Facet> {
    let probe: serde_json::Value = serde_json::from_str(raw)
        .map_err(|err| anyhow::anyhow!("reading the verdict payload: {err}"))?;

    if probe.get("artifacts").is_some() {
        let input: RenderPolicyInput = serde_json::from_str(raw)
            .map_err(|err| anyhow::anyhow!("reading the render-verdict payload: {err}"))?;
        return Ok(Facet {
            role: "render-verdict",
            sources: input.sources,
            settings: input.settings,
        });
    }

    let input: PolicyInput = serde_json::from_str(raw)
        .map_err(|err| anyhow::anyhow!("reading the ir-verdict payload: {err}"))?;
    Ok(Facet {
        role: "ir-verdict",
        sources: input.sources,
        settings: input.settings,
    })
}
