// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

//! The seam: the one module a new adapter rewrites.

use serde_json::value::RawValue;

/// One policy program the attachment carried.
///
/// It is the four roles' source entries normalized to one shape, so the engine
/// behind the seam reads a single type rather than four that differ only in their
/// declaring role.
#[derive(Debug, Clone)]
pub struct Program<'a> {
    /// The file's base name, as the attachment carried it. The host supplies a
    /// base name and never a path from its own filesystem, so the same
    /// attachment yields the same programs wherever it is built.
    pub path: &'a str,
    /// The program's source text, carried verbatim. Nothing outside this guest
    /// has read it.
    pub text: &'a str,
}

/// Runs the policy engine this adapter wraps and returns the decisions it
/// reached under one named decision.
///
/// Replace this body with a call into your engine; everything else in this
/// skeleton can stay as it is.
///
/// The four arguments are the four things an adapter maps:
///
/// - `programs` is the attachment's files, which are the engine's program text.
/// - `settings` is the attachment's settings object as raw JSON. It is the
///   engine's data, and the place to read any tunable the adapter itself needs.
///   It is `None` when the attachment declares no settings.
/// - `payload` is the whole role payload as raw JSON, which is the document the
///   engine evaluates against. Handing over the whole payload rather than a
///   projection of it means a policy reads the graph, the artifacts, the nodes or
///   the targets under the names the wire already gives them, so the input is
///   learnable from one place.
/// - `decision` names which decision the caller wants back. Each role asks for
///   its own; the names are the constants declared in the role modules.
///
/// Each returned element is one decision, as raw JSON. The role that asked for it
/// reads it into the shape that role emits, so the field names a policy author
/// writes are the field names the wire carries and there is no second spelling to
/// keep in step.
///
/// # Two obligations on whatever replaces this body
///
/// Identical input must yield an identical sequence of decisions. A build's
/// output is byte-identical across runs of identical input, and an adapter that
/// iterated an unordered collection would break that from inside the guest. Where
/// an engine yields a set, sort it before returning.
///
/// A failure to evaluate must return an error, never an empty result. An empty
/// result is a policy that found nothing to say; an error is a policy that did
/// not run. Reporting the second as the first lets a build through unexamined.
///
/// # What the placeholder does
///
/// This body reaches no engine and reads no program. It reports back what it was
/// handed, so a freshly built adapter says something at every role rather than
/// being indistinguishable from one that is silently doing nothing: the two
/// transform roles emit no changes, and the verdict roles emit one `info` note.
/// That note is the wiring proof, and it goes with the placeholder.
pub fn evaluate(
    programs: &[Program<'_>],
    settings: Option<&str>,
    payload: &str,
    decision: &str,
) -> anyhow::Result<Vec<Box<RawValue>>> {
    let _ = payload;

    if decision != crate::verdicts::AUDIT_DECISION {
        return Ok(Vec::new());
    }
    let received = if programs.is_empty() {
        "no programs".to_string()
    } else {
        programs
            .iter()
            .map(|p| format!("{} ({} bytes)", p.path, p.text.len()))
            .collect::<Vec<_>>()
            .join(", ")
    };
    let note = format!(
        "this adapter has no policy engine yet: the seam received {received} and {} settings",
        if settings.is_some() { "some" } else { "no" }
    );
    Ok(vec![RawValue::from_string(serde_json::to_string(&note)?)?])
}
