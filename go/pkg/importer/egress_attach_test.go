// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// attachOn is the settings document asking for core/egress-allowlist on each
// drafted platform instance.
const attachOn = `{"attach_egress_allowlist":true}`

// egressReleaseLines are the lines the comment above a drafted platform instance
// keyed key gains when the attachment is drafted: what of it the Release sets.
func egressReleaseLines(key string) string {
	return "    #   - infra.control_plane_present of the egress-allowlist attachment, the affirmation that the cluster's " +
		"network plugin enforces network policy, without which the build halts\n" +
		"    #   - infra.admitted_destinations of the egress-allowlist attachment, an address range for each connection " +
		"whose target the cluster does not run, such as a hosted model, without which the build halts naming that " +
		"connection\n" +
		"    #   - aggrega.ai/namespace on the boundary holding " + key + ", where the target emits its workloads into no " +
		"namespace: the attachment's network policy is placed in its workload's namespace, and the build halts " +
		"without one\n"
}

// egressFeatures is the features block a drafted platform instance keyed key
// opens with when the attachment is drafted.
func egressFeatures(key string) string {
	return "      features:\n" +
		"        # Lets " + key + " connect only to the components it is wired to, through a network policy; compose " +
		"has no network policy, so a compose build halts naming it.\n" +
		"        egress-allowlist:\n" +
		"          type: core/egress-allowlist\n"
}

// egressAttachedMessage is the info the import raises for the platform instance
// keyed key when the attachment is drafted.
func egressAttachedMessage(key string) string {
	return key + " carries core/egress-allowlist, attached as egress-allowlist, which lets it connect only to the " +
		"components it is wired to; the import sets none of its infrastructure: the Release sets " +
		"infra.control_plane_present on that attachment, without which the build halts, and " +
		"infra.admitted_destinations for each connection whose target the cluster does not run"
}

// TestTheAttachEgressAllowlistSettingAttachesTheFeatureToThePlatform imports
// platformSource with attach_egress_allowlist true. The drafted Aggregate is the
// default draft byte for byte but for two additions: the comment above the
// platform's instance names, for the Release, the affirmation, the address ranges
// and the namespace the attachment's realization halts without; and the instance
// opens a features block attaching core/egress-allowlist as egress-allowlist,
// with its type and nothing else — no infra, which an Aggregate's attachment
// does not carry, and so no affirmation. Every drafted Component file is the
// default's. Each wired part's decision says the attachment admits its
// connection, every other decision is the default's, and the import raises one
// info naming the attachment and what the Release sets.
//
// 🔴 PLANTED DEFECTS:
//   - in (*platformDraft).entity (platform.go), replace the condition opening the
//     features block, `if attach {` before `b.WriteString("      features:\n")`,
//     with `if attach && false {`: the draft carries no attachment and this test
//     fails;
//   - in importPlatform (platform.go), replace `egressAllowlistKey, inst.key,
//     w.part.Key)` with `egressAllowlistKey, inst.key, inst.key)`: the two wired
//     decisions name the wrong connection and this test fails.
func TestTheAttachEgressAllowlistSettingAttachesTheFeatureToThePlatform(t *testing.T) {
	t.Parallel()
	def := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(platformSource))
	out := platformImport(t, platformTestFormat(), platformCatalog(), attachOn, oneSource(platformSource))

	instance := "    testplat:\n      type: core/ai-agent/testplat\n"
	lastLine := "    #   - the credential each connection under uses presents to the part it reaches, attached on that " +
		"connection\n"
	want := strings.Replace(renderedPlatformAggregate(), lastLine+instance,
		lastLine+egressReleaseLines("testplat")+instance+egressFeatures("testplat"), 1)
	if want == renderedPlatformAggregate() {
		t.Fatal("the expected draft was not built: the default draft no longer reads as this test expects")
	}
	if len(out.Files) != len(def.Files) {
		t.Fatalf("the import wrote %d files, want the default's %d", len(out.Files), len(def.Files))
	}
	for i, f := range out.Files {
		wantContent := def.Files[i].Content
		if f.Path == "aggregate.yaml" {
			wantContent = want
		}
		if f.Path != def.Files[i].Path || f.Content != wantContent {
			t.Errorf("%s reads\n%s\nwant\n%s", f.Path, f.Content, wantContent)
		}
	}
	agg := out.Files[len(out.Files)-1].Content
	for _, absent := range []string{"infra:", "control_plane_present:", "admitted_destinations:"} {
		if strings.Contains(agg, absent) {
			t.Errorf("the draft writes %q; an Aggregate's attachment carries no infrastructure", absent)
		}
	}

	wantDiags := []aggrega.Diagnostic{{Severity: "info", Code: "feature-attached",
		Message: egressAttachedMessage("testplat")}}
	if !reflect.DeepEqual(out.Diagnostics, wantDiags) {
		t.Errorf("the import raised\n%+v\nwant\n%+v", out.Diagnostics, wantDiags)
	}

	if len(out.Decisions) != len(def.Decisions) {
		t.Fatalf("the import recorded %d decisions, want the default's %d", len(out.Decisions), len(def.Decisions))
	}
	for i, d := range out.Decisions {
		w := def.Decisions[i]
		switch d.SourceNode.ID {
		case "model-1", "finder":
			w.Rationale += "; the egress-allowlist attachment of testplat admits its connection to " + d.SourceNode.ID
		}
		if !reflect.DeepEqual(d, w) {
			t.Errorf("the decision for %s reads\n%+v\nwant\n%+v", d.SourceNode.ID, d, w)
		}
	}
}

// TestTheTargetLinesSitInsideTheFeaturesTheAttachmentOpens drafts a platform
// whose Recipes on one target need an attachment of its own declared, with
// attach_egress_allowlist true and false. With it false the target's lines are
// shown as the default draft shows them, opening a features key of their own.
// With it true they are shown inside the features block the attachment opens,
// with no second features key, so that uncommenting them where they stand leaves
// one features map holding both attachments, each line where the default's
// uncommented line would sit within it.
//
// 🔴 PLANTED DEFECT: in (*platformDraft).entity (platform.go), replace
// `writeTargetAttachment(&b, inst.key, a, attach)` with
// `writeTargetAttachment(&b, inst.key, a, false)`: the draft writes a second
// features key inside the instance and this test fails.
func TestTheTargetLinesSitInsideTheFeaturesTheAttachmentOpens(t *testing.T) {
	t.Parallel()
	format := platformTestFormat()
	format.Platform.TargetAttachments = []TargetAttachment{{Target: "compose", Slot: "admin", Type: "core/secret-value",
		Key: "ADMIN_PASSWORD", Why: "the compose Recipe reads every secret the Release alone attaches under one key"}}
	off := platformImport(t, format, platformCatalog(), `{"attach_egress_allowlist":false}`, oneSource(platformSource))
	on := platformImport(t, format, platformCatalog(), attachOn, oneSource(platformSource))
	offAgg, onAgg := off.Files[len(off.Files)-1].Content, on.Files[len(on.Files)-1].Content

	opening := "# For the compose target only, uncomment the lines below, which declare admin on testplat: the compose " +
		"Recipe reads every secret the Release alone attaches under one key.\n"
	wantOff := "      type: core/ai-agent/testplat\n      " + opening +
		"      # features:\n      #   admin:\n      #     type: core/secret-value\n      #     values:\n" +
		"      #       key: ADMIN_PASSWORD\n      values:\n"
	if !strings.Contains(offAgg, wantOff) {
		t.Errorf("with the setting false the draft does not show the target's lines as the default does:\n%s", offAgg)
	}
	wantOn := "      type: core/ai-agent/testplat\n" + egressFeatures("testplat") + "        " + opening +
		"        # admin:\n        #   type: core/secret-value\n        #   values:\n        #     key: ADMIN_PASSWORD\n" +
		"      values:\n"
	if !strings.Contains(onAgg, wantOn) {
		t.Errorf("with the setting true the draft does not show the target's lines inside the features block:\n%s", onAgg)
	}
	if n := strings.Count(onAgg, "features:"); n != 1 {
		t.Errorf("the draft writes %d features keys, commented or not; want the attachment's one", n)
	}

	// Uncommented where they stand, the lines fill the one features map.
	var uncommented []string
	for _, line := range strings.Split(onAgg, "\n") {
		if strings.HasPrefix(line, "        # ") && !strings.HasPrefix(line, "        # Lets ") &&
			!strings.HasPrefix(line, "        # For the ") {
			uncommented = append(uncommented, "        "+strings.TrimPrefix(line, "        # "))
		}
	}
	wantLines := []string{"        admin:", "          type: core/secret-value", "          values:",
		"            key: ADMIN_PASSWORD"}
	if !reflect.DeepEqual(uncommented, wantLines) {
		t.Errorf("uncommented, the target's lines read\n%s\nwant\n%s", strings.Join(uncommented, "\n"),
			strings.Join(wantLines, "\n"))
	}
}

// TestAnAttachSettingThatIsNotTrueDraftsAsBefore holds every way of not asking
// for the attachment to the draft made with no settings at all, byte for byte —
// files, entities, decisions and diagnostics: the setting absent, false, a
// string, a number and null. A value that is not a JSON boolean reads as false,
// and a setting written after it is still read.
//
// 🔴 PLANTED DEFECT: in parseSettings (settings.go), add
// `s.AttachEgressAllowlist = true` before `return s`: every row fails.
func TestAnAttachSettingThatIsNotTrueDraftsAsBefore(t *testing.T) {
	t.Parallel()
	for _, source := range []string{platformSource, renderSource} {
		for _, tc := range []struct{ settings, baseline string }{
			{settings: `{}`},
			{settings: `{"attach_egress_allowlist":false}`},
			{settings: `{"attach_egress_allowlist":"yes"}`},
			{settings: `{"attach_egress_allowlist":1}`},
			{settings: `{"attach_egress_allowlist":null}`},
			{settings: `{"attach_egress_allowlist":"true","target_namespace":"acme"}`,
				baseline: `{"target_namespace":"acme"}`},
		} {
			base := platformImport(t, platformTestFormat(), platformCatalog(), tc.baseline, oneSource(source))
			got := platformImport(t, platformTestFormat(), platformCatalog(), tc.settings, oneSource(source))
			a, _ := json.Marshal(base)
			b, _ := json.Marshal(got)
			if !bytes.Equal(a, b) {
				t.Errorf("%s drafts differently from %q:\n  got: %s\n want: %s", tc.settings, tc.baseline, b, a)
			}
		}
	}
}

// TestTheNodeByNodeDraftAttachesNothingAndWarns imports with
// attach_egress_allowlist true where the draft is made node by node — a platform
// format under layout nodes, and a format describing no platform. Nothing is
// attached: files, entities and decisions are the same import's without the
// setting, byte for byte, and the one difference is a warning, first among the
// diagnostics, saying nothing was attached and why.
//
// 🔴 PLANTED DEFECT: in Import (import.go), replace
// `if settings.AttachEgressAllowlist {` with `if false {`: both rows fail, the
// warning absent.
func TestTheNodeByNodeDraftAttachesNothingAndWarns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		format        Format
		without, with string
		why           string
	}{
		{name: "layout nodes", format: platformTestFormat(), without: `{"layout":"nodes"}`,
			with: `{"layout":"nodes","attach_egress_allowlist":true}`, why: "the layout setting is nodes"},
		{name: "a format describing no platform", format: testFormat(), without: "", with: attachOn,
			why: "the format describes no platform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			without := platformImport(t, tc.format, platformCatalog(), tc.without, oneSource(platformSource))
			with := platformImport(t, tc.format, platformCatalog(), tc.with, oneSource(platformSource))
			if !reflect.DeepEqual(with.Files, without.Files) || !reflect.DeepEqual(with.Entities, without.Entities) ||
				!reflect.DeepEqual(with.Decisions, without.Decisions) {
				t.Error("the node-by-node draft changed with the setting; it attaches nothing")
			}
			warning := aggrega.Diagnostic{Severity: "warn", Code: "feature-not-attached",
				Message: "the attach_egress_allowlist setting attaches core/egress-allowlist to the platform a draft " +
					"carries the flow in, and this draft writes no platform, since " + tc.why + ", so nothing is attached"}
			want := append([]aggrega.Diagnostic{warning}, without.Diagnostics...)
			if !reflect.DeepEqual(with.Diagnostics, want) {
				t.Errorf("the import raised\n%+v\nwant\n%+v", with.Diagnostics, want)
			}
		})
	}
}

// TestEveryPlatformInstanceCarriesTheAttachment imports two sources with
// attach_egress_allowlist true: each source's platform instance carries the
// attachment under its own key, the comment above each names that instance, and
// the import raises one info per instance, referenced by its source's name.
func TestEveryPlatformInstanceCarriesTheAttachment(t *testing.T) {
	t.Parallel()
	out := platformImport(t, platformTestFormat(), platformCatalog(), attachOn,
		aggrega.ImportSource{Name: "a.json", Bytes: []byte(platformSource)},
		aggrega.ImportSource{Name: "b.json", Bytes: []byte(platformSource)},
	)
	agg := out.Files[len(out.Files)-1].Content
	for _, key := range []string{"testplat", "testplat-2"} {
		if want := "    " + key + ":\n      type: core/ai-agent/testplat\n" + egressFeatures(key); !strings.Contains(agg, want) {
			t.Errorf("the Aggregate does not carry\n%s\nit reads\n%s", want, agg)
		}
		if !strings.Contains(agg, egressReleaseLines(key)) {
			t.Errorf("the comment above %s does not name what the Release sets", key)
		}
	}
	want := []aggrega.Diagnostic{
		{Severity: "info", Code: "feature-attached", Message: egressAttachedMessage("testplat"), SourceRef: "a.json"},
		{Severity: "info", Code: "feature-attached", Message: egressAttachedMessage("testplat-2"), SourceRef: "b.json"},
	}
	if !reflect.DeepEqual(out.Diagnostics, want) {
		t.Errorf("the import raised\n%+v\nwant\n%+v", out.Diagnostics, want)
	}
	if d := decisionOf(t, out, "b.json#model-1"); !strings.HasSuffix(d.Rationale,
		"; the egress-allowlist attachment of testplat-2 admits its connection to model-1-2") {
		t.Errorf("the second instance's model decision reads %q", d.Rationale)
	}
}
