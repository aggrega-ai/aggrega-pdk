// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! The annotation-transform role.
//!
//! # What arrives
//!
//! The components this attachment may annotate, each with the boundary it sits
//! under, its own name and the annotations it already carries; plus the
//! annotation keys the Manifest has registered a meaning for. An annotation
//! under one of those keys goes on to expand into an attached feature, so a
//! policy reads that list to inject keys that will take effect.
//!
//! # What may be emitted
//!
//! Annotations to add to components that already exist. A decision carries a
//! coordinate, a key and a string value, and there is no other field it could
//! carry, so this role cannot express any other change. A decision naming a
//! component the payload does not carry is refused by the build, naming the
//! coordinate.
//!
//! A decision is read straight into the wire's own injection shape, so the field
//! names a policy writes are the field names the build reads.

use aggrega_abi::{AnnotationInjection, AnnotationTransformInput, AnnotationTransformOutput};

use crate::evaluator::{self, Program};

/// The decision this role asks the engine for.
///
/// The name is the adapter's own: the engine behind the seam is told which
/// decision to return, and this is the word a policy author collects annotation
/// decisions under. Change it and the policies written against this adapter
/// change with it, so it is stated once, here.
pub const DECISION: &str = "annotations";

/// Reads the payload, asks the engine for its annotation decisions, and returns
/// the annotations the build adds.
///
/// The payload is read through the generated codec on its type, and written back
/// out as the JSON document the seam reads, under the names the payload declares.
pub fn injections(raw: &[u8]) -> anyhow::Result<AnnotationTransformOutput> {
    let input = AnnotationTransformInput::decode_cbor(raw)
        .map_err(|err| anyhow::anyhow!("reading the annotation-transform payload: {err}"))?;
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
    .map_err(|err| anyhow::anyhow!("evaluating the annotation-transform policy: {err}"))?;

    let mut injections = Vec::with_capacity(decisions.len());
    for decision in &decisions {
        let injection: AnnotationInjection = serde_json::from_str(decision.get()).map_err(|err| {
            anyhow::anyhow!(
                "a decision collected under {DECISION:?} is not an annotation decision: {err}. \
                 A decision names \"boundary_path\", \"component_name\", \"annotation_key\" and \"annotation_value\"."
            )
        })?;
        if injection.annotation_key.is_empty() || injection.component_name.is_empty() {
            anyhow::bail!(
                "a decision collected under {DECISION:?} leaves \"annotation_key\" or \
                 \"component_name\" empty; an annotation with no key, or none to put it on, \
                 is not something a policy meant to ask for"
            );
        }
        injections.push(injection);
    }

    Ok(AnnotationTransformOutput {
        injections,
        ..Default::default()
    })
}
