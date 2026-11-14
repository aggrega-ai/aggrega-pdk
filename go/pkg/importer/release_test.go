// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// aggregateFile returns the content of the drafted Aggregate's file out holds,
// empty where it holds none.
func aggregateFile(out aggrega.ImportOutput) string {
	for _, f := range out.Files {
		if f.Path == "aggregate.yaml" {
			return f.Content
		}
	}
	return ""
}

// withHintsOnly returns the settings document given — "" for none — asking for
// the hints-only reuse mode besides what it asks for, so a case reading what a
// drafted part renders as reads the draft it was written against.
func withHintsOnly(settings string) string {
	if settings == "" {
		return hintsOnly
	}
	return strings.Replace(settings, "{", `{"reuse_mode":"hints-only",`, 1)
}

// draftedAggregate is a drafted Aggregate entity named name, as an importer
// hands DraftRelease the Aggregate it drafted.
func draftedAggregate(name string) aggrega.EmittedEntity {
	return aggrega.EmittedEntity{Kind: "Aggregate", Name: name, Mode: "draft"}
}

// releaseHead is the head every drafted Release of the test's format opens with,
// named name, its manifest declared empty beneath the comment saying why.
func releaseHead(name string) string {
	return "apiVersion: aggrega.ai/v1beta1\n" +
		"kind: Release\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  # environment is a drafted default, since no source names the environment this Release deploys to; " +
		"change it to name the one it deploys to.\n" +
		"  environment: development\n" +
		"  version: 0.1.0\n" +
		"  description: Draft Release imported from a source in the testflow format.\n" +
		"spec:\n" +
		"  aggregate: " + name + "\n" +
		"  # manifest: left empty, since the import was given no target; an import given a target drafts the " +
		"Manifest and names it here.\n" +
		"  manifest: \"\"\n"
}

// TestDraftReleaseWritesTheReleaseOfTheDraft pins, byte for byte, the Release
// DraftRelease writes, one row per shape a draft hands it: named as the name it
// is handed; its environment development beneath the comment saying it is a
// drafted default; manifest declared empty beneath the comment saying why, the
// two lines the host replaces with the drafted Manifest's name; the parameters
// the draft leaves, each empty beneath a comment; and the entries — each with
// what it leaves for the Release that names no key above it, under one heading,
// written "<path>, <text>" or either alone; then its comments, a comment holding
// a line feed written as two comment lines and an empty one as a bare hash; and
// its body beneath its key: its lines, then its values, each left item naming a
// key as that key holding an empty string beneath its comment, written into a
// mapping its lines already write, and its secrets — one whose key its lines
// write, or whose path runs through a list, named above the entry instead; an
// entry with nothing to write as an empty mapping — under components, or, where
// one is a module, under the one boundary the draft names, the instances under
// its components and the modules under its aggregates, each in the order given.
// Every row is written twice, and must write the same bytes both times.
//
// 🔴 PLANTED DEFECTS, each in DraftRelease (release.go):
//
//   - drop the line writing the environment: every row fails;
//   - write every entry under components whatever its Module says: the module
//     rows fail;
//   - in place, write the node where the last segment's key is already written
//     instead of reporting false: the "already written" row fails with two
//     image keys.
func TestDraftReleaseWritesTheReleaseOfTheDraft(t *testing.T) {
	t.Parallel()
	in := aggrega.ImportInput{Format: testFormatName, Name: "support.json"}
	for _, row := range []struct {
		scenario string
		in       aggrega.ImportInput
		name     string
		draft    ReleaseDraft
		want     string
	}{
		{"no entry", in, "imported/support", ReleaseDraft{},
			releaseHead("imported/support") + "  components: {}\n"},
		{"entries with comments and lines", in, "imported/support", ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "api", Left: []LeftItem{{Path: "infra.port", Text: "the port it listens on"}, {Path: "infra.region"},
				{Text: "the value of its token"}}, Comments: []string{"The service api."},
				Lines: []string{"infra:", "  image: \"nginx:1\"", "features:", "  token:", "    type: core/secret-value"}},
			{Key: "cache"},
			{Key: "db", Comments: []string{"two\nlines", ""}},
		}},
			releaseHead("imported/support") +
				"  components:\n" +
				"    # Left for the Release, since the source cannot supply it:\n" +
				"    #   - the value of its token\n" +
				"    # The service api.\n" +
				"    api:\n" +
				"      infra:\n" +
				"        image: \"nginx:1\"\n" +
				"        # Left for the Release, since the source cannot supply it: infra.port, the port it listens on\n" +
				"        port: \"\"\n" +
				"        # Left for the Release, since the source cannot supply it: infra.region\n" +
				"        region: \"\"\n" +
				"      features:\n" +
				"        token:\n" +
				"          type: core/secret-value\n" +
				"    cache: {}\n" +
				"    # two\n" +
				"    # lines\n" +
				"    #\n" +
				"    db: {}\n"},
		{"already written, or under a list", in, "imported/support", ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "srv", Left: []LeftItem{LeftImage("since it names none"), {Path: "infra.command.0", Text: "written empty"},
				{Path: "infra.port.number", Text: "under a value"}},
				Lines: []string{"infra:", "  image: \"x\"", "  port: 80", "  command:", "    - \"\""}},
		}},
			releaseHead("imported/support") +
				"  components:\n" +
				"    # Left for the Release, since the source cannot supply it:\n" +
				"    #   - infra.image, since it names none\n" +
				"    #   - infra.command.0, written empty\n" +
				"    #   - infra.port.number, under a value\n" +
				"    srv:\n" +
				"      infra:\n" +
				"        image: \"x\"\n" +
				"        port: 80\n" +
				"        command:\n" +
				"          - \"\"\n"},
		{"values and secrets", in, "imported/support", ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "flow", Values: []ReleaseValue{{Path: "infra.image", Value: `"flow:1"`, Comment: "The image."}},
				Left:    []LeftItem{{Path: "infra.owner", Text: "the owner"}},
				Secrets: []Secret{{Path: "features.password", Variable: "FLOW_PASSWORD", Text: "the password"}}},
		}},
			releaseHead("imported/support") +
				"  components:\n" +
				"    flow:\n" +
				"      infra:\n" +
				"        # The image.\n" +
				"        image: \"flow:1\"\n" +
				"        # Left for the Release, since the source cannot supply it: infra.owner, the owner\n" +
				"        owner: \"\"\n" +
				"      features:\n" +
				"        password:\n" +
				"          type: core/secret-value\n" +
				"          infra:\n" +
				"            # Attached by reference, never as a value: the password, read from the variable " +
				"FLOW_PASSWORD, which a target substituting variable references, as Docker Compose does when the stack " +
				"starts, reads from the environment, so no file carries the value.\n" +
				"            origin: \"${FLOW_PASSWORD}\"\n"},
		{"a title and a namespace", aggrega.ImportInput{Format: testFormatName, Name: "support.json",
			Settings: json.RawMessage(`{"target_namespace":"team-a"}`)}, "team-a/support-chat",
			ReleaseDraft{Entries: []ReleaseEntry{{Key: "chat"}}},
			releaseHead("team-a/support-chat") + "  components:\n    chat: {}\n"},
		{"a module alone, a parameter left", in, "imported/support", ReleaseDraft{Boundary: "platform",
			Parameters: []LeftItem{{Path: "key", Text: "the key"}},
			Entries: []ReleaseEntry{
				{Key: "dify", Module: true, Comments: []string{"The installation."}},
			}},
			releaseHead("imported/support") +
				"  parameters:\n" +
				"    # Left for the Release, since the source cannot supply it: key, the key\n" +
				"    key: \"\"\n" +
				"  boundaries:\n" +
				"    platform:\n" +
				"      aggregates:\n" +
				"        # The installation.\n" +
				"        dify: {}\n"},
		{"instances and a module, no boundary named", in, "imported/support", ReleaseDraft{Entries: []ReleaseEntry{
			{Key: "web", Left: []LeftItem{LeftImage("the image running it")}},
			{Key: "mod", Module: true, Lines: []string{"components:", "  api: {}"}},
			{Key: "db"},
		}},
			releaseHead("imported/support") +
				"  boundaries:\n" +
				"    default:\n" +
				"      components:\n" +
				"        web:\n" +
				"          infra:\n" +
				"            # Left for the Release, since the source cannot supply it: infra.image, the image running it\n" +
				"            image: \"\"\n" +
				"        db: {}\n" +
				"      aggregates:\n" +
				"        mod:\n" +
				"          components:\n" +
				"            api: {}\n"},
	} {
		first := DraftRelease(row.in, draftedAggregate(row.name), row.draft)
		again := DraftRelease(row.in, draftedAggregate(row.name), row.draft)
		if len(first) != 1 || len(again) != 1 {
			t.Fatalf("%s: DraftRelease returned %d and %d entities, want the Release alone", row.scenario, len(first),
				len(again))
		}
		if first[0].Body != row.want {
			t.Errorf("%s: DraftRelease wrote\n%s\nwant\n%s", row.scenario, first[0].Body, row.want)
		}
		if again[0].Body != first[0].Body {
			t.Errorf("%s: DraftRelease wrote two byte sequences:\n%s\nand\n%s", row.scenario, first[0].Body,
				again[0].Body)
		}
		if r := first[0]; r.Kind != "Release" || r.Mode != "draft" || r.Name != row.name {
			t.Errorf("%s: DraftRelease returned the %s %s %q, want the drafted Release %q", row.scenario, r.Mode,
				r.Kind, r.Name, row.name)
		}
	}
}

// TestASecretIsWrittenForTheTargetAsAReference pins how DraftRelease writes one
// secret, a connection's credential, for each target an import names, and for
// none: as core/secret-value whose origin is the variable reference ${VARIABLE}
// for compose and for no target, and as core/referenced-secret whose origin is
// written empty — the Secret's name, left for the Release — for helm, a draft
// then also writing a Manifest mapping core/referenced-secret to
// helm/k8s-existing-secret; a secret marked Referenced is attached as
// core/referenced-secret for every target, its origin the variable reference
// for compose and the Secret's name for helm, the Manifest drafted only for
// helm. No row writes a value: the only origin written is a variable reference
// or an empty string.
//
// 🔴 PLANTED DEFECT: in Secret.Form (secret.go), return the referenced form for
// a Referenced secret ahead of the helm case: the referenced helm row fails,
// writing a variable reference and no Manifest.
func TestASecretIsWrittenForTheTargetAsAReference(t *testing.T) {
	t.Parallel()
	credential := ConnectionCredential("flow", "model", "gpt", "gpt", "OPENAI_API_KEY")
	byVariable := func(typ string) string {
		return "      uses:\n" +
			"        model:\n" +
			"          gpt:\n" +
			"            features:\n" +
			"              credential:\n" +
			"                type: " + typ + "\n" +
			"                infra:\n" +
			"                  # Attached by reference, never as a value: the credential flow presents to gpt on " +
			"its model connection, read from the variable OPENAI_API_KEY, which a target substituting variable " +
			"references, as Docker Compose does when the stack starts, reads from the environment, so no file " +
			"carries the value.\n" +
			"                  origin: \"${OPENAI_API_KEY}\"\n"
	}
	byValue := byVariable("core/secret-value")
	inHelm := "      uses:\n" +
		"        model:\n" +
		"          gpt:\n" +
		"            features:\n" +
		"              credential:\n" +
		"                type: core/referenced-secret\n" +
		"                infra:\n" +
		"                  # Left for the Release, since the source cannot supply it: the name of a Kubernetes " +
		"Secret already in the namespace holding, under the data key value, the credential flow presents to gpt on " +
		"its model connection.\n" +
		"                  origin: \"\"\n"
	manifest := "apiVersion: aggrega.ai/v1beta1\n" +
		"kind: Manifest\n" +
		"metadata:\n" +
		"  name: imported/flow\n" +
		"  version: 0.1.0\n" +
		"spec:\n" +
		"  mappings:\n" +
		"    features:\n" +
		"      type:\n" +
		"        core/referenced-secret: helm/k8s-existing-secret\n"
	marked := credential
	marked.Referenced = true
	for _, row := range []struct {
		scenario, target string
		secret           Secret
		wantBody         string
		wantManifest     string
	}{
		{"compose", "compose", credential, byValue, ""},
		{"no target", "", credential, byValue, ""},
		{"helm", "helm", credential, inHelm, manifest},
		{"referenced, compose", "compose", marked, byVariable("core/referenced-secret"), ""},
		{"referenced, no target", "", marked, byVariable("core/referenced-secret"), ""},
		{"referenced, helm", "helm", marked, inHelm, manifest},
	} {
		in := aggrega.ImportInput{Format: testFormatName, Target: row.target}
		out := DraftRelease(in, draftedAggregate("imported/flow"), ReleaseDraft{Entries: []ReleaseEntry{{Key: "flow",
			Secrets: []Secret{row.secret}}}})
		want := releaseHead("imported/flow") + "  components:\n    flow:\n" + row.wantBody
		if out[0].Body != want {
			t.Errorf("%s: DraftRelease wrote\n%s\nwant\n%s", row.scenario, out[0].Body, want)
		}
		switch {
		case row.wantManifest == "" && len(out) != 1:
			t.Errorf("%s: DraftRelease returned %d entities, want the Release alone", row.scenario, len(out))
		case row.wantManifest != "" && (len(out) != 2 || out[1].Kind != "Manifest" || out[1].Mode != "draft" ||
			out[1].Name != "imported/flow" || out[1].Body != row.wantManifest):
			t.Errorf("%s: DraftRelease returned %+v, want the Release and the Manifest\n%s", row.scenario, out,
				row.wantManifest)
		}
	}
}

// TestASecretParameterIsWrittenForTheTarget pins how DraftRelease writes a
// parameter holding a secret, after a parameter left empty, for each target an
// import names, and for none: for compose and for none, holding the variable
// reference ${SIGNING_KEY} beneath the comment saying it is passed by reference,
// with no Manifest; for helm, holding an empty string — the Secret's name, owed —
// beneath the comment a left parameter is written with, the draft also writing a
// Manifest mapping core/referenced-secret to helm/k8s-existing-secret, though no
// entry attaches a secret.
//
// 🔴 PLANTED DEFECT: in DraftRelease (release.go), drop the loop handing each
// secret parameter to realize: the helm row fails, no Manifest written.
func TestASecretParameterIsWrittenForTheTarget(t *testing.T) {
	t.Parallel()
	left := "  parameters:\n" +
		"    # Left for the Release, since the source cannot supply it: region, the region\n" +
		"    region: \"\"\n"
	byVariable := left +
		"    # Passed by reference, never as a value: signing_key, the key the platform signs with; it is read from " +
		"the variable SIGNING_KEY, which a target substituting variable references, as Docker Compose does when the " +
		"stack starts, reads from the environment, so no file carries the value.\n" +
		"    signing_key: \"${SIGNING_KEY}\"\n"
	byName := left +
		"    # Left for the Release, since the source cannot supply it: signing_key, the key the platform signs with\n" +
		"    signing_key: \"\"\n"
	manifest := "apiVersion: aggrega.ai/v1beta1\n" +
		"kind: Manifest\n" +
		"metadata:\n" +
		"  name: imported/flow\n" +
		"  version: 0.1.0\n" +
		"spec:\n" +
		"  mappings:\n" +
		"    features:\n" +
		"      type:\n" +
		"        core/referenced-secret: helm/k8s-existing-secret\n"
	for _, row := range []struct {
		scenario, target string
		wantParameters   string
		wantManifest     string
	}{
		{"compose", "compose", byVariable, ""},
		{"no target", "", byVariable, ""},
		{"helm", "helm", byName, manifest},
	} {
		in := aggrega.ImportInput{Format: testFormatName, Target: row.target}
		out := DraftRelease(in, draftedAggregate("imported/flow"), ReleaseDraft{
			Parameters: []LeftItem{{Path: "region", Text: "the region"}},
			SecretParameters: []Secret{{Path: "signing_key", Variable: "SIGNING_KEY",
				Text: "the key the platform signs with", Referenced: true}},
			Entries: []ReleaseEntry{{Key: "flow"}}})
		want := releaseHead("imported/flow") + row.wantParameters + "  components:\n    flow: {}\n"
		if out[0].Body != want {
			t.Errorf("%s: DraftRelease wrote\n%s\nwant\n%s", row.scenario, out[0].Body, want)
		}
		switch {
		case row.wantManifest == "" && len(out) != 1:
			t.Errorf("%s: DraftRelease returned %d entities, want the Release alone", row.scenario, len(out))
		case row.wantManifest != "" && (len(out) != 2 || out[1].Kind != "Manifest" || out[1].Body != row.wantManifest):
			t.Errorf("%s: DraftRelease returned %+v, want the Release and the Manifest\n%s", row.scenario, out,
				row.wantManifest)
		}
	}
}

// TestASetAndANamedRealizationAreWrittenForTheTarget pins how DraftRelease writes
// a secret set — a server's environment, one variable holding a variable
// reference and one holding a value still owed — and an entry naming the Recipe
// realizing its instance: for compose and for no target, the set as
// core/referenced-secret-set with its entries as given; for helm, as
// core/referenced-secret named by a Secret holding one data key per variable,
// listed by name, its value never written. Given a target, the Manifest maps the
// instance by its key to <target>/<realization>, and, for helm, the secret type
// to helm/k8s-existing-secret; given none, no Manifest is drafted.
//
// 🔴 PLANTED DEFECT: in DraftRelease (release.go), drop `&& in.Target != ""`: the
// no-target row fails, a Manifest mapping the instance to "/mcp-stdio-bridge".
func TestASetAndANamedRealizationAreWrittenForTheTarget(t *testing.T) {
	t.Parallel()
	set := Secret{Path: "features.server_env", Text: "the environment github is started with",
		Entries: []SecretEntry{{Key: "TOKEN", Value: "${GITHUB_TOKEN}"}, {Key: "OWNER", Value: ""}}}
	asEntries := "      features:\n" +
		"        server_env:\n" +
		"          type: core/referenced-secret-set\n" +
		"          infra:\n" +
		"            # Attached by reference wherever the source names a variable: the environment github is " +
		"started with, one entry per variable, each written as the source writes it, so a variable reference stays " +
		"one, which a target substituting variable references, as Docker Compose does when the stack starts, reads " +
		"from the environment, and an empty value is one still owed.\n" +
		"            entries:\n" +
		"              - key: \"TOKEN\"\n" +
		"                value: \"${GITHUB_TOKEN}\"\n" +
		"              - key: \"OWNER\"\n" +
		"                value: \"\"\n"
	inHelm := "      features:\n" +
		"        server_env:\n" +
		"          type: core/referenced-secret\n" +
		"          infra:\n" +
		"            # Left for the Release, since the source cannot supply it: the name of a Kubernetes Secret " +
		"already in the namespace holding the environment github is started with, one data key per variable, each " +
		"named as the variable it holds: TOKEN, OWNER.\n" +
		"            origin: \"\"\n"
	manifest := func(target, features string) string {
		return "apiVersion: aggrega.ai/v1beta1\n" +
			"kind: Manifest\n" +
			"metadata:\n" +
			"  name: imported/mcp\n" +
			"  version: 0.1.0\n" +
			"spec:\n" +
			"  mappings:\n" +
			"    components:\n" +
			"      name:\n" +
			"        github: " + target + "/mcp-stdio-bridge\n" + features
	}
	for _, row := range []struct {
		scenario, target, wantBody, wantManifest string
	}{
		{"compose", "compose", asEntries, manifest("compose", "")},
		{"no target", "", asEntries, ""},
		{"helm", "helm", inHelm, manifest("helm", "    features:\n      type:\n"+
			"        core/referenced-secret: helm/k8s-existing-secret\n")},
	} {
		in := aggrega.ImportInput{Format: testFormatName, Target: row.target}
		out := DraftRelease(in, draftedAggregate("imported/mcp"), ReleaseDraft{Entries: []ReleaseEntry{{Key: "github",
			Realization: "mcp-stdio-bridge", Secrets: []Secret{set}}}})
		if want := releaseHead("imported/mcp") + "  components:\n    github:\n" + row.wantBody; out[0].Body != want {
			t.Errorf("%s: DraftRelease wrote\n%s\nwant\n%s", row.scenario, out[0].Body, want)
		}
		switch {
		case row.wantManifest == "" && len(out) != 1:
			t.Errorf("%s: DraftRelease returned %d entities, want the Release alone", row.scenario, len(out))
		case row.wantManifest != "" && (len(out) != 2 || out[1].Body != row.wantManifest):
			t.Errorf("%s: DraftRelease returned %+v, want the Release and the Manifest\n%s", row.scenario, out,
				row.wantManifest)
		}
	}
}

// TestConnectionCredentialNamesTheCredentialOnItsConnection pins the secret an
// entry attaches for the credential its instance presents on one connection: the
// path the Release attaches it at under that connection, the variable it is read
// from — the one given, or, given none, the target's VariableName followed by
// _CREDENTIAL — and the sentence naming the instance, the target and the socket.
func TestConnectionCredentialNamesTheCredentialOnItsConnection(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		scenario                             string
		key, socket, alias, target, variable string
		want                                 Secret
	}{
		{"an alias naming its target", "agent", "model", "gpt-4o", "gpt-4o", "OPENAI_API_KEY",
			Secret{Path: "uses.model.gpt-4o.features.credential", Variable: "OPENAI_API_KEY",
				Text: "the credential agent presents to gpt-4o on its model connection"}},
		{"an alias apart from its target, no variable", "flow", "tools", "docs", "docs-search", "",
			Secret{Path: "uses.tools.docs.features.credential", Variable: "DOCS_SEARCH_CREDENTIAL",
				Text: "the credential flow presents to docs-search on its tools connection"}},
	} {
		if got := ConnectionCredential(row.key, row.socket, row.alias, row.target, row.variable); !reflect.DeepEqual(got,
			row.want) {
			t.Errorf("%s: ConnectionCredential wrote %+v, want %+v", row.scenario, got, row.want)
		}
	}
}

// TestRenderWritesTheDraftedReleaseBesideTheAggregate hands Render an importer's
// own entities — a drafted Component, a drafted Aggregate, and the Release and
// the Manifest DraftRelease wrote for the helm target — and requires the Release
// written as release.yaml and the Manifest as manifest.yaml, after the
// Aggregate, as the entities give them; under render_mode llm no draft file is
// written, the Release's included.
func TestRenderWritesTheDraftedReleaseBesideTheAggregate(t *testing.T) {
	t.Parallel()
	in := aggrega.ImportInput{Format: testFormatName, Target: "helm"}
	drafted := DraftRelease(in, draftedAggregate("imported/flow"), ReleaseDraft{Entries: []ReleaseEntry{{Key: "a",
		Secrets: []Secret{{Path: "features.key", Variable: "KEY", Text: "the key"}}}}})
	if len(drafted) != 2 {
		t.Fatalf("DraftRelease returned %d entities for a helm draft attaching a secret, want the Release and the "+
			"Manifest", len(drafted))
	}
	entities := append([]aggrega.EmittedEntity{
		{Kind: "Component", Name: "imported/component/a", Mode: "draft", Body: "kind: Component\n"},
		{Kind: "Aggregate", Name: "imported/flow", Mode: "draft", Body: "kind: Aggregate\n"},
	}, drafted...)
	var paths []string
	for _, f := range Render(nil, entities, nil) {
		paths = append(paths, f.Path)
		if f.Path == ReleaseFileName && f.Content != drafted[0].Body {
			t.Errorf("%s reads\n%s\nwant the Release's body\n%s", f.Path, f.Content, drafted[0].Body)
		}
		if f.Path == ManifestFileName && f.Content != drafted[1].Body {
			t.Errorf("%s reads\n%s\nwant the Manifest's body\n%s", f.Path, f.Content, drafted[1].Body)
		}
	}
	if want := []string{"components/imported-component-a.yaml", "aggregate.yaml", ReleaseFileName,
		ManifestFileName}; !slices.Equal(paths, want) {
		t.Errorf("Render wrote %v, want %v", paths, want)
	}
	for _, f := range Render(json.RawMessage(`{"render_mode":"llm"}`), entities, nil) {
		if f.Path == ReleaseFileName || f.Path == ManifestFileName {
			t.Errorf("render_mode llm wrote %s", f.Path)
		}
	}
}

// releaseWriter is the file holding the package's one Release writer.
const releaseWriter = "release.go"

// releaseComposedApart is one place a source composes a Release other than
// through the package's writer: where, and how.
type releaseComposedApart struct {
	at, how string
}

// releasesComposedApart parses one Go source and returns every place it composes
// a Release itself: a string literal holding a Release's kind line, or an
// EmittedEntity literal — written with its package, bare, or elided inside a
// composite of them — whose Kind is Release and whose Mode is not reference.
func releasesComposedApart(t *testing.T, at string, src []byte) []releaseComposedApart {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, at, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", at, err)
	}
	var out []releaseComposedApart
	ast.Inspect(file, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING && strings.Contains(literalText(e), "kind: Release") {
				out = append(out, releaseComposedApart{at: at + ":" + strconv.Itoa(fset.Position(e.Pos()).Line),
					how: "a string literal writing a Release's kind"})
			}
		case *ast.CompositeLit:
			if !emittedEntityType(e.Type) {
				return true
			}
			fields := map[string]ast.Expr{}
			for _, elt := range e.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = kv.Value
					}
				}
			}
			if literalText(fields["Kind"]) == "Release" && literalText(fields["Mode"]) != "reference" {
				out = append(out, releaseComposedApart{at: at + ":" + strconv.Itoa(fset.Position(e.Pos()).Line),
					how: "an EmittedEntity of kind Release"})
			}
		}
		return true
	})
	return out
}

// TestEveryReleaseOfThePackageIsWrittenByDraftRelease parses the package's own
// source and holds every Release it drafts to its one writer: outside release.go
// no string literal writes a Release's kind line and no EmittedEntity literal is
// a drafted Release, and inside it — where both are the writer's own — exactly
// one EmittedEntity literal is, so a scan that read nothing fails. The check is first run over a planted source
// holding each form, each required to be found, and a call of DraftRelease,
// required to pass.
//
// The compiler's importers are held to the same rule by a twin check,
// TestEveryImporterDraftsItsReleaseThroughTheKit in the compiler's
// plugins/internal/guestwire: this package is a module of its own, published
// apart, and imports nothing from the compiler's module, so the two checks are
// written once on each side.
//
// 🔴 PLANTED DEFECT: in (*platformDraft).release (platform.go), return
// []aggrega.EmittedEntity{{Kind: "Release", Name: aggregate, Mode: "draft", Body:
// "kind: Release\n"}}: this test reddens naming platform.go twice, for the
// literal and for the kind line.
func TestEveryReleaseOfThePackageIsWrittenByDraftRelease(t *testing.T) {
	t.Parallel()
	const planted = `package p

func (a *b) text() string {
	return "apiVersion: v\nkind: Release\n"
}

func (a *b) selected(in aggrega.ImportInput) aggrega.EmittedEntity {
	return aggrega.EmittedEntity{Kind: "Release", Name: draftName(a.settings, in, nil), Mode: "draft"}
}

func (a *b) elided(in aggrega.ImportInput) []EmittedEntity {
	return []EmittedEntity{{Kind: "Release", Name: draftName(a.settings, in, nil), Mode: "draft"}}
}

func (a *b) written(in aggrega.ImportInput) []aggrega.EmittedEntity {
	return DraftRelease(in, a.aggregate, ReleaseDraft{})
}
`
	var hows []string
	for _, c := range releasesComposedApart(t, "planted.go", []byte(planted)) {
		hows = append(hows, c.how)
	}
	if want := []string{"a string literal writing a Release's kind", "an EmittedEntity of kind Release",
		"an EmittedEntity of kind Release"}; !slices.Equal(hows, want) {
		t.Fatalf("the check found %v in the planted source; want the kind line and both literals, and nothing for "+
			"the call of DraftRelease", hows)
	}

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob the package's source: %v", err)
	}
	sort.Strings(paths)
	written := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, c := range releasesComposedApart(t, path, src) {
			if path == releaseWriter {
				if c.how == "an EmittedEntity of kind Release" {
					written++
				}
				continue
			}
			t.Errorf("%s composes a Release with %s; every Release the package drafts is written by DraftRelease, in "+
				"%s", c.at, c.how, releaseWriter)
		}
	}
	if written != 1 {
		t.Errorf("found %d drafted Release literal(s) in %s, want DraftRelease's one; the scan is not reading what it "+
			"checks", written, releaseWriter)
	}
}
