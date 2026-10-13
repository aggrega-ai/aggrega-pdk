// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! The label-transform role.
//!
//! # What arrives
//!
//! The build's nodes keyed by name, each carrying the component it instantiates,
//! the version resolved for it and the labels it currently has; plus the
//! Release's name and environment, so a policy can decide differently per
//! environment.
//!
//! # What may be emitted
//!
//! Label changes, one per decision. Every instruction this module builds
//! addresses one label on one node, so a decision has no way to reach the node's
//! component, its resolved version, the set of nodes, or a value that is not a
//! string. That confinement is the whole of what an operator invited by attaching
//! a plugin here.
//!
//! # How it travels
//!
//! This role's wire carries the binary encoding in both directions, as every
//! role's does, so the payload is read and the result written through the
//! generated codec on the payload type. The seam reads a JSON document, so the
//! decoded payload is written back out as one before it is handed over — the
//! names a policy addresses are the payload's own names.
//!
//! # A policy names the label; this module builds the path
//!
//! The change is expressed to the build as a document of edit instructions
//! addressed by path, and those paths need escaping — a label key routinely
//! contains a `/`, which is the path separator, so a hand-written path would
//! address a nested location and the change would be refused. A decision
//! therefore names the label it means and this module addresses it. The escaping
//! is written once, here, rather than in every policy.

use aggrega_abi::{MutatorInput, MutatorOutput};
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;

use crate::evaluator::{self, Program};

/// The decision this role asks the engine for. See
/// [`crate::annotations::DECISION`] for why the name is the adapter's to choose
/// and is stated once.
pub const DECISION: &str = "labels";

/// The two operations a label decision may ask for.
const OP_ADD: &str = "add";
const OP_REMOVE: &str = "remove";

/// One label decision as a policy expresses it.
///
/// A field name the decision does not declare is refused rather than ignored: a
/// misspelled `value` would otherwise read as a decision with no value at all,
/// which is the kind of silent miss a policy author cannot see.
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct Decision {
    /// The name of the node the decision applies to.
    node: String,
    /// The label key the decision is about.
    key: String,
    /// The value the label takes. Required when adding, unused when dropping.
    #[serde(default)]
    value: Option<String>,
    /// Whether the label is added or dropped. Absent means added.
    #[serde(default)]
    op: Option<String>,
}

/// One edit instruction, in the shape the build reads.
#[derive(Debug, Serialize)]
struct Instruction {
    op: String,
    path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    value: Option<String>,
}

/// Reads the payload, asks the engine for its label decisions, and returns the
/// label changes the build applies.
pub fn changes(raw: &[u8]) -> anyhow::Result<MutatorOutput> {
    let input = MutatorInput::decode_cbor(raw)
        .map_err(|err| anyhow::anyhow!("reading the label-transform payload: {err}"))?;
    let payload = serde_json::to_string(&input)?;

    let programs: Vec<Program<'_>> = input
        .sources
        .iter()
        .map(|s| Program {
            path: &s.path,
            text: &s.content,
        })
        .collect();

    let decisions = evaluator::evaluate(
        &programs,
        input.settings.as_ref().map(|s| s.get()),
        &payload,
        DECISION,
    )
    .map_err(|err| anyhow::anyhow!("evaluating the label-transform policy: {err}"))?;

    let mut patches = Vec::with_capacity(decisions.len());
    for decision in &decisions {
        let decision: Decision = serde_json::from_str(decision.get()).map_err(|err| {
            anyhow::anyhow!(
                "a decision collected under {DECISION:?} is not a label decision: {err}. \
                 A decision names \"node\", \"key\" and \"value\", and may name \"op\" as \
                 {OP_ADD:?} or {OP_REMOVE:?}."
            )
        })?;
        patches.push(instruction(&decision)?);
    }

    Ok(MutatorOutput {
        patches,
        ..Default::default()
    })
}

/// Builds the single-operation edit document one decision asks for.
fn instruction(decision: &Decision) -> anyhow::Result<Box<RawValue>> {
    let op = decision.op.as_deref().unwrap_or(OP_ADD);
    let value = match op {
        OP_ADD => Some(decision.value.clone().ok_or_else(|| {
            anyhow::anyhow!(
                "the {DECISION:?} decision for node {:?} label {:?} names no \"value\"; \
                 a label value is a string",
                decision.node,
                decision.key
            )
        })?),
        OP_REMOVE => None,
        other => anyhow::bail!(
            "the {DECISION:?} decision for node {:?} label {:?} names \"op\" {other:?}, \
             which is not one of {OP_ADD:?} or {OP_REMOVE:?}",
            decision.node,
            decision.key
        ),
    };
    let instruction = Instruction {
        op: op.to_string(),
        path: format!(
            "/nodes/{}/labels/{}",
            escape(&decision.node),
            escape(&decision.key)
        ),
        value,
    };
    Ok(RawValue::from_string(serde_json::to_string(&[
        instruction
    ])?)?)
}

/// Escapes one path segment so a name containing the path separator addresses a
/// single location rather than a nested one.
///
/// The escape character is itself escaped first, so a separator rewritten into an
/// escape sequence is not escaped twice.
fn escape(segment: &str) -> String {
    segment.replace('~', "~0").replace('/', "~1")
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The canonical label keys carry a `/`, so this is the ordinary case rather
    /// than an edge case: unescaped it would address a nested location and the
    /// change would be refused.
    #[test]
    fn a_label_key_carrying_the_path_separator_addresses_one_label() {
        let document = instruction(&Decision {
            node: "main/api".to_string(),
            key: "component.aggrega.ai/type".to_string(),
            value: Some("server".to_string()),
            op: None,
        })
        .expect("the decision is read");
        assert_eq!(
            document.get(),
            r#"[{"op":"add","path":"/nodes/main~1api/labels/component.aggrega.ai~1type","value":"server"}]"#
        );
    }

    #[test]
    fn an_escape_character_in_a_name_is_escaped_first() {
        let document = instruction(&Decision {
            node: "a~b".to_string(),
            key: "c/d".to_string(),
            value: Some("v".to_string()),
            op: None,
        })
        .expect("the decision is read");
        assert!(
            document.get().contains(r#""path":"/nodes/a~0b/labels/c~1d""#),
            "{}",
            document.get()
        );
    }

    #[test]
    fn dropping_a_label_carries_no_value() {
        let document = instruction(&Decision {
            node: "api".to_string(),
            key: "scratch".to_string(),
            value: None,
            op: Some(OP_REMOVE.to_string()),
        })
        .expect("the decision is read");
        assert_eq!(
            document.get(),
            r#"[{"op":"remove","path":"/nodes/api/labels/scratch"}]"#
        );
    }

    #[test]
    fn adding_a_label_with_no_value_is_refused() {
        let err = instruction(&Decision {
            node: "api".to_string(),
            key: "tier".to_string(),
            value: None,
            op: None,
        })
        .expect_err("adding with no value is refused");
        assert!(err.to_string().contains("value"), "{err}");
    }

    #[test]
    fn an_unknown_operation_is_refused() {
        let err = instruction(&Decision {
            node: "api".to_string(),
            key: "tier".to_string(),
            value: Some("gold".to_string()),
            op: Some("replace".to_string()),
        })
        .expect_err("an operation outside the two is refused");
        let message = err.to_string();
        assert!(message.contains("replace"), "{message}");
        assert!(message.contains(OP_ADD), "{message}");
    }
}
