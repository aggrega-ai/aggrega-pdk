// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// TestAnInstancesDeclarationsOpenOneFeaturesKey pins DeclarationsText over an
// instance attaching two secrets on its own slots, one on a connection and one
// set: for compose the two own secrets declared live under one features key,
// opened above the first, none for the connection's or the set; with the
// instance already opening features, no second key; with no target each shown
// commented out; for helm nothing.
//
// 🔴 PLANTED DEFECT: in DeclarationsText (secret.go), drop the features.<slot>
// filter: the compose row declares the connection's credential on the instance.
func TestAnInstancesDeclarationsOpenOneFeaturesKey(t *testing.T) {
	t.Parallel()
	secrets := []Secret{
		{Path: "features.password", Variable: "DB_PASSWORD", Key: "POSTGRES_PASSWORD", Text: "the password"},
		{Path: "uses.backend.db.features.credential", Variable: "DB_PASSWORD", Text: "the credential"},
		{Path: "features.server_env", Text: "the environment", Entries: []SecretEntry{{Key: "A", Value: "${A}"}}},
		{Path: "features.token", Variable: "TOKEN", Text: "the token"},
	}
	why := DeclarationWhy + ".\n"
	live := "      features:\n" +
		"        # For the compose target, the lines below declare password on db: " + why +
		"        password:\n          type: core/secret-value\n          values:\n            key: POSTGRES_PASSWORD\n" +
		"        # For the compose target, the lines below declare token on db: " + why +
		"        token:\n          type: core/secret-value\n          values:\n            key: TOKEN\n"
	for _, row := range []struct {
		scenario, target string
		inFeatures       bool
		want             string
	}{
		{"compose", "compose", false, live},
		{"compose, features already open", "compose", true, strings.TrimPrefix(live, "      features:\n")},
		{"helm", "helm", false, ""},
	} {
		if got := DeclarationsText(row.target, "      ", "db", secrets, row.inFeatures); got != row.want {
			t.Errorf("%s: DeclarationsText wrote\n%s\nwant\n%s", row.scenario, got, row.want)
		}
	}
	commented := DeclarationsText("", "      ", "db", secrets, false)
	if strings.Count(commented, "      # features:\n") != 2 || strings.Contains(commented, "\n      features:\n") ||
		strings.Contains(commented, "credential") || strings.Contains(commented, "server_env") {
		t.Errorf("with no target the declarations are not each shown commented out, the connection's and the set's "+
			"left out:\n%s", commented)
	}
}

// TestASecretsFormAndDeclarationAreDecidedOncePerTarget pins Secret.Form and
// Secret.Declaration, the one place a drafted secret's form is decided, for each
// target a request can carry and for none, for a secret keyed apart from its
// variable and for one keyed by its variable, and for one the catalog attaches
// by reference:
//
//   - compose: core/secret-value by variable reference, keyed, no realization;
//     declared live with its key;
//   - helm: core/referenced-secret named by its Secret, mapped to
//     helm/k8s-existing-secret; nothing declared;
//   - no target: core/secret-value by variable reference, not keyed; compose's
//     lines declared, not live;
//   - another target: core/secret-value by variable reference, not keyed;
//     nothing declared;
//   - a Referenced secret: core/referenced-secret whatever the target, by
//     variable reference for compose and for none, named by its Secret and
//     mapped for helm; never declared;
//   - a set: core/referenced-secret-set for compose, for none and for another
//     target, and core/referenced-secret named by its Secret, mapped, for helm;
//     never declared.
//
// 🔴 PLANTED DEFECT: in Secret.Form (secret.go), drop the helm case: the helm
// rows fail, the secret written by variable reference. Move the Entries case or
// the Referenced case above the helm case: the helm set row, or the referenced
// helm row, fails, written as its entries or by variable reference on helm.
func TestASecretsFormAndDeclarationAreDecidedOncePerTarget(t *testing.T) {
	t.Parallel()
	keyed := Secret{Path: "features.admin", Variable: "APP_ADMIN", Key: "ADMIN_KEY", Text: "the password"}
	byVariable := Secret{Path: "uses.model.m.features.credential", Variable: "OPENAI_API_KEY", Text: "the key"}
	referenced := Secret{Path: "components.api.features.owner", Variable: "OWNER", Text: "the owner", Referenced: true}
	set := Secret{Path: "features.server_env", Text: "the environment",
		Entries: []SecretEntry{{Key: "TOKEN", Value: "${GITHUB_TOKEN}"}}}
	setForm := SecretForm{Type: "core/referenced-secret-set"}
	adminLines := []string{"admin:", "  type: core/secret-value", "  values:", "    key: ADMIN_KEY"}
	credentialLines := []string{"credential:", "  type: core/secret-value", "  values:", "    key: OPENAI_API_KEY"}
	value := SecretForm{Type: "core/secret-value"}
	keyedValue := SecretForm{Type: "core/secret-value", Keyed: true}
	referencedValue := SecretForm{Type: "core/referenced-secret"}
	byNameOnHelm := SecretForm{Type: "core/referenced-secret", ByName: true, Realization: "helm/k8s-existing-secret"}
	for _, row := range []struct {
		scenario, target string
		secret           Secret
		form             SecretForm
		declaration      SecretDeclaration
	}{
		{"compose, keyed apart", "compose", keyed, keyedValue,
			SecretDeclaration{Lines: adminLines, Target: "compose", Live: true, Slot: "admin"}},
		{"compose, keyed by its variable", "compose", byVariable, keyedValue,
			SecretDeclaration{Lines: credentialLines, Target: "compose", Live: true, Slot: "credential"}},
		{"helm", "helm", keyed, byNameOnHelm, SecretDeclaration{}},
		{"no target", "", keyed, value, SecretDeclaration{Lines: adminLines, Target: "compose", Slot: "admin"}},
		{"another target", "cloudformation", keyed, value, SecretDeclaration{}},
		{"referenced, compose", "compose", referenced, referencedValue, SecretDeclaration{}},
		{"referenced, no target", "", referenced, referencedValue, SecretDeclaration{}},
		{"referenced, helm", "helm", referenced, byNameOnHelm, SecretDeclaration{}},
		{"a set, compose", "compose", set, setForm, SecretDeclaration{}},
		{"a set, no target", "", set, setForm, SecretDeclaration{}},
		{"a set, another target", "cloudformation", set, setForm, SecretDeclaration{}},
		{"a set, helm", "helm", set, byNameOnHelm, SecretDeclaration{}},
	} {
		if got := row.secret.Form(row.target); got != row.form {
			t.Errorf("%s: Form is %+v, want %+v", row.scenario, got, row.form)
		}
		if got := row.secret.Declaration(row.target); !reflect.DeepEqual(got, row.declaration) {
			t.Errorf("%s: Declaration is %+v, want %+v", row.scenario, got, row.declaration)
		}
	}
}

// TestSecretFormOnArm pins Secret.Form for arm: every secret is
// core/referenced-secret realized by arm/key-vault-secret, named by its resource
// (the origin left for the Release) unless the source states an Origin; a set
// stays a set.
//
// 🔴 PLANTED DEFECT: in Secret.form (secret.go), drop the arm case: the value row
// fails, core/secret-value written by variable reference.
func TestSecretFormOnArm(t *testing.T) {
	t.Parallel()
	value := Secret{Path: "features.password", Variable: "DB_PASSWORD", Text: "the password"}
	byReference := SecretForm{Type: "core/referenced-secret", ByName: true, Realization: "arm/key-vault-secret"}
	located := SecretForm{Type: "core/referenced-secret", Realization: "arm/key-vault-secret"}
	referenced := value
	referenced.Referenced = true
	stated := value
	stated.Origin = "/subscriptions/s/resourceGroups/g/providers/Microsoft.KeyVault/vaults/v/secrets/p"
	set := Secret{Path: "features.env", Entries: []SecretEntry{{Key: "A", Value: "${A}"}}}
	for _, row := range []struct {
		scenario string
		secret   Secret
		want     SecretForm
	}{
		{"a value", value, byReference},
		{"referenced", referenced, byReference},
		{"with an origin", stated, located},
		{"a set", set, SecretForm{Type: "core/referenced-secret-set"}},
	} {
		if got := row.secret.Form("arm"); got != row.want {
			t.Errorf("%s: Form is %+v, want %+v", row.scenario, got, row.want)
		}
		if decl := row.secret.Declaration("arm"); len(decl.Lines) != 0 {
			t.Errorf("%s: Declaration is %+v, want none", row.scenario, decl)
		}
	}
}

// TestSecretFormOnCloudFormation pins Secret.Form for cloudformation: a Referenced
// secret, or one stating an Origin, is core/referenced-secret; any other is
// core/secret-value; neither names a realization.
//
// 🔴 PLANTED DEFECT: in Secret.form (secret.go), drop the Origin case: the origin
// row fails, core/secret-value.
func TestSecretFormOnCloudFormation(t *testing.T) {
	t.Parallel()
	value := Secret{Path: "features.password", Variable: "DB_PASSWORD", Text: "the password"}
	referenced, stated := value, value
	referenced.Referenced = true
	stated.Origin = "arn:aws:secretsmanager:eu-west-1:1:secret:p"
	for _, row := range []struct {
		scenario string
		secret   Secret
		want     SecretForm
	}{
		{"a value", value, SecretForm{Type: "core/secret-value"}},
		{"referenced", referenced, SecretForm{Type: "core/referenced-secret"}},
		{"with an origin", stated, SecretForm{Type: "core/referenced-secret"}},
	} {
		if got := row.secret.Form("cloudformation"); got != row.want {
			t.Errorf("%s: Form is %+v, want %+v", row.scenario, got, row.want)
		}
	}
}

// TestSecretFormOnTerraform pins Secret.Form for terraform: a value is
// core/secret-value realized by terraform/variable-secret, keyed, its origin the
// variable reference; a Referenced secret is core/referenced-secret named by its
// resource, realized only by what Secret.Realization names.
//
// 🔴 PLANTED DEFECT: in Secret.form (secret.go), drop terraform from the keyed
// targets: the value row fails, and its key is not declared.
func TestSecretFormOnTerraform(t *testing.T) {
	t.Parallel()
	value := Secret{Path: "features.password", Variable: "DB_PASSWORD", Text: "the password"}
	referenced, named := value, value
	referenced.Referenced = true
	named.Referenced, named.Realization = true, "terraform/key-vault-secret"
	withOrigin := value
	withOrigin.Origin = "projects/p/secrets/s"
	for _, row := range []struct {
		scenario string
		secret   Secret
		want     SecretForm
	}{
		{"a value", value, SecretForm{Type: "core/secret-value", Keyed: true, Realization: "terraform/variable-secret"}},
		{"referenced", referenced, SecretForm{Type: "core/referenced-secret", ByName: true}},
		{"an origin, not referenced", withOrigin, SecretForm{Type: "core/referenced-secret"}},
		{"referenced, realization named", named, SecretForm{Type: "core/referenced-secret", ByName: true,
			Realization: "terraform/key-vault-secret"}},
	} {
		if got := row.secret.Form("terraform"); got != row.want {
			t.Errorf("%s: Form is %+v, want %+v", row.scenario, got, row.want)
		}
	}
	if decl := value.Declaration("terraform"); !decl.Live || decl.Target != "terraform" || decl.Slot != "password" {
		t.Errorf("Declaration is %+v, want the key declared live for terraform", decl)
	}
}

// TestSecretRealizationOverridesTheForm pins that Secret.Realization is the
// form's Realization on every target, and that the Manifest DraftRelease writes
// maps the secret's type to it.
//
// 🔴 PLANTED DEFECT: in Secret.Form (secret.go), drop the override: every row fails.
func TestSecretRealizationOverridesTheForm(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"", "helm", "compose", "arm", "cloudformation", "terraform"} {
		s := Secret{Path: "features.password", Variable: "P", Text: "the password", Referenced: true,
			Realization: "chosen-secret"}
		form := s.Form(target)
		if form.Realization != "chosen-secret" {
			t.Errorf("%q: Form realization is %q, want the secret's own", target, form.Realization)
		}
		out := DraftRelease(aggrega.ImportInput{Format: testFormatName, Target: target}, draftedAggregate("imported/flow"),
			ReleaseDraft{Entries: []ReleaseEntry{{Key: "flow", Secrets: []Secret{s}}}})
		want := "        " + form.Type + ": chosen-secret\n"
		if len(out) != 2 || !strings.HasSuffix(out[1].Body, want) {
			t.Errorf("%q: DraftRelease wrote %+v, want a Manifest ending %q", target, out, want)
		}
	}
}

// TestADraftedReleaseWritesAKnownOriginOrAsksForOne pins the origin a Release
// writes on arm: the locator a secret's Origin states, as it is; otherwise an
// empty origin beneath a comment asking for the Key Vault secret's resource ID and
// naming the parameter whose value goes into it.
//
// 🔴 PLANTED DEFECT: in secretNode (secret.go), drop the Origin case: the first
// row writes an empty origin.
func TestADraftedReleaseWritesAKnownOriginOrAsksForOne(t *testing.T) {
	t.Parallel()
	id := "/subscriptions/s/resourceGroups/g/providers/Microsoft.KeyVault/vaults/v/secrets/p"
	for _, row := range []struct {
		scenario, origin, want string
	}{
		{"known", id, `origin: "` + id + `"`},
		{"asked", "", `origin: ""`},
	} {
		s := Secret{Path: "features.password", Variable: "DB_PASSWORD", Text: "the password", Origin: row.origin}
		out := DraftRelease(aggrega.ImportInput{Format: testFormatName, Target: "arm"}, draftedAggregate("imported/flow"),
			ReleaseDraft{Entries: []ReleaseEntry{{Key: "flow", Secrets: []Secret{s}}}})
		body := out[0].Body
		if !strings.Contains(body, row.want+"\n") {
			t.Errorf("%s: Release lacks %q:\n%s", row.scenario, row.want, body)
		}
		asks := strings.Contains(body, "the resource ID of the Azure Key Vault secret holding the password") &&
			strings.Contains(body, "the template parameter DB_PASSWORD")
		if asks != (row.origin == "") {
			t.Errorf("%s: the comment asking for the resource ID is present: %v", row.scenario, asks)
		}
	}
}

// TestAnOriginIsIgnoredWhereTheTargetReadsNone pins that helm, compose and an
// import naming no target write a secret exactly as they do without an Origin.
//
// 🔴 PLANTED DEFECT: in Secret.locator (secret.go), return Origin for every
// target: the helm form row fails.
func TestAnOriginIsIgnoredWhereTheTargetReadsNone(t *testing.T) {
	t.Parallel()
	plain := Secret{Path: "features.password", Variable: "P", Text: "the password"}
	stated := plain
	stated.Origin = "/subscriptions/s/resourceGroups/g/providers/Microsoft.KeyVault/vaults/v/secrets/p"
	for _, target := range []string{"helm", "compose", ""} {
		if got, want := stated.Form(target), plain.Form(target); got != want {
			t.Errorf("%q: Form with an Origin is %+v, want %+v", target, got, want)
		}
		in := aggrega.ImportInput{Format: testFormatName, Target: target}
		draft := func(s Secret) string {
			return DraftRelease(in, draftedAggregate("imported/flow"),
				ReleaseDraft{Entries: []ReleaseEntry{{Key: "flow", Secrets: []Secret{s}}}})[0].Body
		}
		if got, want := draft(stated), draft(plain); got != want {
			t.Errorf("%q: the Release with an Origin is\n%s\nwant\n%s", target, got, want)
		}
	}
}
