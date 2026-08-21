// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! Adapter skeleton for a governance plugin: copy this directory, drop your
//! policy engine into the seam, and the result is a guest an operator can attach
//! at every governance role.
//!
//! This is a skeleton, not a framework. No policy engine is compiled in and no
//! policy language is assumed. Everything specific to an engine lives behind one
//! function, [`evaluator::evaluate`], which is the one module a new adapter has
//! to rewrite. The role modules around it — [`annotations`], [`labels`],
//! [`verdicts`] — are the parts that stay the same whatever engine is behind the
//! seam: each decodes the payload its role receives, hands it to the seam, and
//! encodes the output its role is allowed to emit.
//!
//! # The four roles this guest serves
//!
//! A governance plugin is one guest attached at one of four roles, and the role
//! an operator attaches it at — not anything the guest says about itself — fixes
//! which payload arrives and what the guest may emit. All four are served here:
//!
//! | Role | Export | What arrives |
//! | --- | --- | --- |
//! | `annotation-transform` | `transform_annotations` | the components that may be annotated |
//! | `label-transform` | `mutate` | the build's nodes and their labels |
//! | `ir-verdict` | `validate` | the build graph |
//! | `render-verdict` | `validate` | the rendered output files |
//!
//! One `validate` export serves both verdict roles. The two payloads identify
//! themselves by shape, so the guest reads what it was handed rather than
//! declaring which role it expects.
//!
//! # The four things an adapter maps
//!
//! Whatever engine sits behind the seam, an adapter is wiring four things:
//!
//! - sources become the engine's program text. The attachment's files arrive
//!   verbatim; the host does not parse them.
//! - settings become the engine's data, and any translation the adapter itself
//!   needs. The attachment's settings object arrives verbatim too.
//! - the input is the payload of the attached role, listed in the table above.
//! - the output is either the verdicts the engine reached, or the changes a
//!   transform role asks for.
//!
//! # Build
//!
//! ```text
//! cargo build --release --target wasm32-wasip1 -p governance-adapter-skeleton-rust
//! ```

//! # How a failure travels
//!
//! A role that cannot read its payload, or whose engine could not evaluate,
//! propagates the failure out of its export and the build reads it as a failed
//! invocation. That is one of two channels: the other is the `error` member every
//! role's output carries, for an adapter that can distinguish "evaluated, and here
//! is what went wrong" from "could not run at all" and wants the first reported
//! alongside a partial result. This skeleton takes one channel rather than both,
//! because two paths to one outcome is two places to keep in step. What neither
//! channel permits is reporting a failure as an empty result, which on a
//! governance surface reads as a policy that allowed the build.

mod annotations;
mod evaluator;
mod labels;
mod verdicts;

use aggrega_pdk_rust::{
    describe, PluginDescriptor, PluginDescriptorMeta, PluginExport, API_VERSION,
};
use extism_pdk::{plugin_fn, FnResult};

/// The self-report the host reads once when it loads the guest.
///
/// It declares the three exports the four governance roles dispatch to. The host
/// refuses to call an export the guest did not declare, so an export is declared
/// here in the same change that implements it — an attachment pointed at a
/// missing export is then refused by name rather than silently doing nothing.
///
/// An adapter serving fewer roles declares fewer exports and deletes the role
/// modules for the roles it does not serve. It declares no capabilities: a governance plugin
/// is not selected by capability, and what it may emit comes from its role.
///
/// `settings_schema` is left unset, so the attachment's settings pass through
/// verbatim. An adapter whose engine needs a settings shape can declare a JSON
/// Schema document there; the host then validates the operator's settings against
/// it before dispatch, and settings that do not conform halt the build.
fn descriptor() -> PluginDescriptor {
    PluginDescriptor {
        abi_version: API_VERSION.to_string(),
        plugin_type: "governance".to_string(),
        description: "Adapter skeleton: wire a policy engine to the governance roles.".to_string(),
        exports: vec![
            PluginExport {
                name: "transform_annotations".to_string(),
                description: "Report the annotations the deployment's components should carry."
                    .to_string(),
                ..Default::default()
            },
            PluginExport {
                name: "mutate".to_string(),
                description: "Report the labels the build's nodes should carry.".to_string(),
                ..Default::default()
            },
            PluginExport {
                name: "validate".to_string(),
                description: "Report the verdicts the policy reached.".to_string(),
                ..Default::default()
            },
        ],
        metadata: Some(Box::new(PluginDescriptorMeta {
            license: "Apache-2.0".to_string(),
            ..Default::default()
        })),
        ..Default::default()
    }
}

#[plugin_fn]
pub fn describe() -> FnResult<String> {
    Ok(describe!(descriptor()))
}

/// Serves the annotation-transform role.
#[plugin_fn]
pub fn transform_annotations(input: String) -> FnResult<String> {
    Ok(serde_json::to_string(&annotations::injections(&input)?)?)
}

/// Serves the label-transform role.
#[plugin_fn]
pub fn mutate(input: String) -> FnResult<String> {
    Ok(serde_json::to_string(&labels::changes(&input)?)?)
}

/// Serves both verdict roles.
#[plugin_fn]
pub fn validate(input: String) -> FnResult<String> {
    Ok(serde_json::to_string(&verdicts::verdicts(&input)?)?)
}
