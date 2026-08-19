// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Attachment-reasoning verdict plugin for the Aggrega Wasm runtime.
//
// It attaches at a verdict role and reasons over the ATTACHMENTS the IR graph
// carries, rather than over node labels or rendered content. Every fact it
// reads comes off the wire: each attachment's verbatim catalog labels, its
// verbatim catalog annotations, whether the attachment fills a declared slot or
// is free, and the sensitivity marking at each of the units the wire carries it
// at — one answer for the whole attachment, one per inject delivery key, and one
// per authored leaf. The host supplies those facts and makes no judgement about
// them; each rule below is this plugin's own judgement.
//
// Five rules, selected by the attachment's settings.rule:
//
//   - no-free-attachments — deny a build carrying an attachment that fills no
//     declared slot. This is the rule an operator writes when free-form
//     attachment is unacceptable in their estate.
//
//   - no-sensitive-by-value — deny a build that carries a declared-sensitive
//     datum by value, by EITHER of the two routes the wire marks per datum: an
//     inject delivery key whose referenced provides key the Feature declares
//     sensitive, or a leaf of the attachment's own authored payload that the
//     Feature declares sensitive. The verdict names which route it found and
//     which datum, because the two are separate findings and an operator acting
//     on one needs to know which one it has. Delivery keys are examined before
//     authored leaves, so a build offending on both is reported at the narrower
//     finding — a marked delivery key names the datum AND where it travels,
//     where an authored leaf names only the datum.
//
//     Neither route needs the value inspected, and neither is inspected. At a
//     marked delivery key the compiler already raises a build-succeeding
//     advisory and never halts; at an authored leaf it raises nothing at all,
//     because whether the author wrote the material there or a locator pointing
//     at it elsewhere is the author's choice and the compiler resolves neither.
//     This rule refuses the carry rather than the content, so it does not need
//     to tell them apart. Turning either route into a refusal is an operator's
//     choice, expressed here.
//
//   - deny-secret-kind — deny a build carrying an attachment whose catalog
//     labels mark it as handling secret material. The judgement is computed
//     from the labels alone, which is what makes a label-carrying wire a
//     superset of a host-computed credential verdict.
//
//   - deny-unreviewed-attachment — deny a build carrying an attachment whose
//     attached Feature's catalog annotations say its review is outstanding.
//     This is the annotations counterpart of deny-secret-kind, and it reads a
//     key in a namespace of the catalog author's own choosing, which is the
//     only kind of key that can show the annotations arrived unfiltered.
//
//   - observe — emit one info verdict per attachment reporting every fact
//     the guest decoded, so a payload can be read at the point it arrives.
//
// settings.echo_label, when set, appends that label's key and value to the
// verdict message. A rule whose message quotes a label the host has no reason
// to know about is how the verbatim-label contract is observed rather than
// assumed: a filtered or prefix-stripped label set could not answer.
//
// settings.severity overrides the verdict severity (halt by default), so the
// same plugin exercises the halting and non-halting host paths.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o governance-attachment-facts-go.wasm ./
package main

import (
	"encoding/json"
	"fmt"
	"sort"

	pdk "github.com/extism/go-pdk"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// input is the narrow projection of the IR-stage verdict payload this guest
// reads: the graph's per-node attachment map and the attachment settings. The
// full payload also carries nodes, edges and precalculated paths; the decoder
// drops the fields these rules do not read.
type input struct {
	Graph struct {
		Spec struct {
			FeatureValuesMapping map[string]map[string]attachment `json:"feature_values_mapping"`
		} `json:"spec"`
	} `json:"graph"`
	Settings struct {
		Rule      string `json:"rule"`
		Severity  string `json:"severity"`
		EchoLabel string `json:"echo_label"`
	} `json:"settings"`
}

// attachment is one entry of the graph's per-node attachment map, projected to
// the facts these rules judge. Labels are the attached Feature's catalog labels
// as the catalog author wrote them; CatalogAnnotations are that same entity's
// catalog annotations, also as written; Binding is the declared slot the
// attachment fills or "free".
//
// The three sensitivity fields are the same axis at three different units, and
// none is derivable from the others. Sensitive is one answer for the whole
// attachment: true when the Feature declares sensitivity anywhere, or when the
// slot it fills does. SensitiveInject maps a delivery mechanism to the delivery
// keys carrying declared-sensitive material, valued by the provides key each
// inherited its marking from. SensitiveAuthoredLeaves names the leaves of the
// attachment's own authored payload that the Feature declares sensitive, each as
// the pillar-prefixed path the datum occupies. A rule wanting per-datum
// precision reads one of the latter two: Sensitive says the attachment handles
// secret material somewhere, which is not a statement about any one datum, so
// treating it as one would object to data an author left readable.
//
// An empty sensitivity field is never permission. It says the marking found
// nothing at that unit, not that the attachment carries nothing sensitive.
//
// CatalogAnnotations is held as raw wire bytes rather than decoded into a map,
// which is deliberate and is about ORDER. The wire carries the annotations in the
// order the catalog entity declares them; a Go map has no order to hold, so
// decoding on arrival would destroy exactly the property the wire promises and
// the observe rule below could no longer report it. Rules that need one key's
// value decode locally instead.
//
// SensitiveAuthoredLeaves is a slice for the same reason read the other way: the
// wire carries the leaves in the order the attachment authored them, and a slice
// keeps that order, so nothing has to be re-established on arrival and the
// observe rule reports the order the payload actually had.
type attachment struct {
	Type                    string                       `json:"type"`
	Labels                  map[string]string            `json:"labels"`
	CatalogAnnotations      json.RawMessage              `json:"catalog_annotations,omitempty"`
	Binding                 string                       `json:"binding"`
	Sensitive               bool                         `json:"sensitive"`
	SensitiveInject         map[string]map[string]string `json:"sensitive_inject"`
	SensitiveAuthoredLeaves []string                     `json:"sensitive_authored_leaves"`
}

type result struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type output struct {
	Results []result `json:"results"`
}

// bindingFree is the value an attachment's binding carries when it fills no
// declared slot on its host Component.
//
// A guest re-spells the wire's literals rather than importing them: the socket
// boundary carries JSON, not Go types, and a plugin written in another language
// has the same three literals in its own source. So this is a wire-contract
// constant restated on the far side of a boundary, not a second home for one
// definition — do not "unify" it with anything on the host side.
const bindingFree = "free"

// secretKindLabel and secretKindValue are the catalog label the standard
// library marks a credential Feature family with. Reading them here is the
// point: the judgement is the plugin's, computed from labels the host carries
// without interpreting.
const (
	secretKindLabel = "feature.aggrega.ai/kind"
	secretKindValue = "secret"
)

// reviewAnnotation and reviewOutstandingValue are a catalog annotation in a
// namespace the fixture catalog author invented. Nothing in the compiler knows
// this key exists, which is what makes a rule keyed on it evidence that the
// annotations crossed the boundary verbatim: a rule keyed on a reserved-namespace
// key would keep firing even if the host filtered the set down to its own
// namespace.
const (
	reviewAnnotation       = "governance.example/review-ticket"
	reviewOutstandingValue = "none"
)

// descriptor is the static describe() self-report: a v1alpha1 governance plugin
// whose validate export the host validates before dispatch.
func descriptor() aggrega.PluginDescriptor {
	return aggrega.PluginDescriptor{
		AbiVersion:  aggrega.APIVersion,
		PluginType:  "governance",
		Description: "Judge a build from each attachment's catalog labels, declared-slot binding and sensitivity marking.",
		Exports:     []aggrega.PluginExport{{Name: "validate"}},
		Metadata:    &aggrega.PluginDescriptorMeta{Author: "aggrega", License: "Apache-2.0"},
	}
}

//go:wasmexport describe
func describe() int32 {
	out, err := aggrega.MarshalDescriptor(descriptor())
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(out)
	return 0
}

//go:wasmexport validate
func validate() int32 {
	var in input
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		pdk.SetError(fmt.Errorf("governance-attachment-facts: unmarshal PolicyInput: %w", err))
		return 1
	}

	severity := in.Settings.Severity
	if severity == "" {
		severity = "halt"
	}

	// An empty results set is the allowing answer: a rule that finds nothing to
	// object to emits no verdict and the build proceeds.
	results := []result{}

	// Walk nodes and attachments in sorted order so the first offender a
	// message names is the same across runs of identical input. The wire
	// carries the attachments in author order; this container does not keep
	// it, so the order is re-established here rather than assumed.
walk:
	for _, nodeID := range sortedKeys(in.Graph.Spec.FeatureValuesMapping) {
		attachments := in.Graph.Spec.FeatureValuesMapping[nodeID]
		for _, name := range sortedKeys(attachments) {
			a := attachments[name]
			switch in.Settings.Rule {
			case "no-free-attachments":
				if a.Binding == bindingFree {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q (type %q) fills no declared slot (binding %q); free-form attachment is not permitted%s",
						nodeID, name, a.Type, a.Binding, echo(a, in.Settings.EchoLabel))})
					break walk
				}
			case "no-sensitive-by-value":
				if mech, deliveryKey, providesKey, found := firstMarkedDelivery(a); found {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q delivers declared-sensitive material by value through %s key %q (from provides key %q)%s",
						nodeID, name, mech, deliveryKey, providesKey, echo(a, in.Settings.EchoLabel))})
					break walk
				}
				if leaf, found := firstMarkedAuthoredLeaf(a); found {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q carries declared-sensitive material by value at authored leaf %q%s",
						nodeID, name, leaf, echo(a, in.Settings.EchoLabel))})
					break walk
				}
			case "deny-secret-kind":
				if a.Labels[secretKindLabel] == secretKindValue {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q handles secret material per its catalog label %s=%s%s",
						nodeID, name, secretKindLabel, secretKindValue, echo(a, in.Settings.EchoLabel))})
					break walk
				}
			case "deny-unreviewed-attachment":
				if annotationValue(a, reviewAnnotation) == reviewOutstandingValue {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q has an outstanding review per its catalog annotation %s=%s; an unreviewed attachment is not permitted%s",
						nodeID, name, reviewAnnotation, reviewOutstandingValue, echo(a, in.Settings.EchoLabel))})
					break walk
				}
			case "observe":
				facts, err := json.Marshal(a)
				if err != nil {
					facts = []byte(`"<unencodable>"`)
				}
				results = append(results, result{Severity: "info", Message: fmt.Sprintf(
					"attachment facts: node %q attachment %q: %s", nodeID, name, facts)})
			}
		}
	}

	emit(output{Results: results})
	return 0
}

// firstMarkedDelivery returns the first marked delivery key on an attachment in
// sorted mechanism-then-key order, with the provides key it inherited its
// marking from.
func firstMarkedDelivery(a attachment) (mechanism, deliveryKey, providesKey string, found bool) {
	for _, mech := range sortedKeys(a.SensitiveInject) {
		for _, key := range sortedKeys(a.SensitiveInject[mech]) {
			return mech, key, a.SensitiveInject[mech][key], true
		}
	}
	return "", "", "", false
}

// firstMarkedAuthoredLeaf returns the attachment's first marked authored leaf,
// as the pillar-prefixed coordinate the wire carries.
//
// No sort here, unlike firstMarkedDelivery above, and the difference is the
// container rather than an inconsistency: the wire carries these leaves in a
// sequence and this decoded them into a slice, so the payload's own order
// survived arrival and the first element is already the first leaf the
// attachment authored. The delivery marking arrives as nested objects decoded
// into Go maps, which hold no order, so there the order has to be
// re-established.
func firstMarkedAuthoredLeaf(a attachment) (leaf string, found bool) {
	if len(a.SensitiveAuthoredLeaves) == 0 {
		return "", false
	}
	return a.SensitiveAuthoredLeaves[0], true
}

// annotationValue decodes the attachment's verbatim catalog annotations and
// returns one key's value, or the empty string when the attachment carries no
// annotations, when the bytes do not decode, or when the key is absent. A rule
// keyed on a specific value therefore cannot fire on an attachment that carried
// nothing.
func annotationValue(a attachment, key string) string {
	if len(a.CatalogAnnotations) == 0 {
		return ""
	}
	var annotations map[string]string
	if err := json.Unmarshal(a.CatalogAnnotations, &annotations); err != nil {
		return ""
	}
	return annotations[key]
}

// echo renders the requested label's key and value for the verdict message. A
// label the guest was not given cannot be echoed, so an empty rendering is the
// honest report that the label did not arrive.
func echo(a attachment, key string) string {
	if key == "" {
		return ""
	}
	value, ok := a.Labels[key]
	if !ok {
		return fmt.Sprintf(" [label %s absent]", key)
	}
	return fmt.Sprintf(" [label %s=%s]", key, value)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func emit(o output) {
	b, err := json.Marshal(o)
	if err != nil {
		pdk.OutputString(`{"results":[{"severity":"halt","message":"fixture failed to encode output"}]}`)
		return
	}
	pdk.Output(b)
}

func main() {}
