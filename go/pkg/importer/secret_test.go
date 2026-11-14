// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"reflect"
	"strings"
	"testing"
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
//   - a Referenced secret: core/referenced-secret whatever the target, mapped only
//     for helm, never declared;
//   - a set: core/referenced-secret-set for compose, for none and for another
//     target, and core/referenced-secret named by its Secret, mapped, for helm;
//     never declared.
//
// 🔴 PLANTED DEFECT: in Secret.Form (secret.go), drop `|| target == helmTarget`:
// the helm rows fail, the secret written by variable reference. Move the
// Entries check above the helm check: the helm set row fails, the set written
// as its entries on helm.
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
	byName := SecretForm{Type: "core/referenced-secret", ByName: true}
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
		{"another target", "terraform", keyed, value, SecretDeclaration{}},
		{"referenced, compose", "compose", referenced, byName, SecretDeclaration{}},
		{"referenced, no target", "", referenced, byName, SecretDeclaration{}},
		{"referenced, helm", "helm", referenced, byNameOnHelm, SecretDeclaration{}},
		{"a set, compose", "compose", set, setForm, SecretDeclaration{}},
		{"a set, no target", "", set, setForm, SecretDeclaration{}},
		{"a set, another target", "terraform", set, setForm, SecretDeclaration{}},
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
