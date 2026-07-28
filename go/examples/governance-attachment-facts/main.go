// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Attachment-reasoning verdict plugin for the Aggrega Wasm runtime.
//
// It attaches at a verdict role and reasons over the ATTACHMENTS the IR graph
// carries, rather than over node labels or rendered content. Every fact it
// reads comes off the wire: each attachment's verbatim catalog labels, whether
// the attachment fills a declared slot or is free, and the per-delivery-key
// sensitivity marking. The host supplies those facts and makes no judgement
// about them; each rule below is this plugin's own judgement.
//
// Four rules, selected by the attachment's settings.rule:
//
//   - no-free-attachments — deny a build carrying an attachment that fills no
//     declared slot. This is the rule an operator writes when free-form
//     attachment is unacceptable in their estate.
//   - no-sensitive-by-value — deny a build where a declared-sensitive datum
//     travels through a by-value delivery key. The compiler warns about this
//     and never halts; turning the warning into a refusal is an operator's
//     choice, expressed here.
//   - deny-secret-kind — deny a build carrying an attachment whose catalog
//     labels mark it as handling secret material. The judgement is computed
//     from the labels alone, which is what makes a label-carrying wire a
//     superset of a host-computed credential verdict.
//   - observe — emit one audit verdict per attachment reporting every fact
//     the guest decoded, so a payload can be read at the point it arrives.
//
// settings.echo_label, when set, appends that label's key and value to the
// verdict message. A rule whose message quotes a label the host has no reason
// to know about is how the verbatim-label contract is observed rather than
// assumed: a filtered or prefix-stripped label set could not answer.
//
// settings.severity overrides the verdict severity (deny by default), so the
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
// as the catalog author wrote them; Binding is the declared slot the attachment
// fills or "free"; SensitiveInject maps a delivery mechanism to the delivery
// keys carrying declared-sensitive material, valued by the provides key each
// inherited its marking from.
type attachment struct {
	Type            string                       `json:"type"`
	Labels          map[string]string            `json:"labels"`
	Binding         string                       `json:"binding"`
	Sensitive       bool                         `json:"sensitive"`
	SensitiveInject map[string]map[string]string `json:"sensitive_inject"`
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
		severity = "deny"
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
			case "deny-secret-kind":
				if a.Labels[secretKindLabel] == secretKindValue {
					results = append(results, result{Severity: severity, Message: fmt.Sprintf(
						"governance rule: node %q attachment %q handles secret material per its catalog label %s=%s%s",
						nodeID, name, secretKindLabel, secretKindValue, echo(a, in.Settings.EchoLabel))})
					break walk
				}
			case "observe":
				facts, err := json.Marshal(a)
				if err != nil {
					facts = []byte(`"<unencodable>"`)
				}
				results = append(results, result{Severity: "audit", Message: fmt.Sprintf(
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
		pdk.OutputString(`{"results":[{"severity":"deny","message":"fixture failed to encode output"}]}`)
		return
	}
	pdk.Output(b)
}

func main() {}
