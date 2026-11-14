// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"reflect"
	"testing"
)

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
//     for helm, never declared.
//
// 🔴 PLANTED DEFECT: in Secret.Form (secret.go), drop `|| target == helmTarget`:
// the helm rows fail, the secret written by variable reference.
func TestASecretsFormAndDeclarationAreDecidedOncePerTarget(t *testing.T) {
	t.Parallel()
	keyed := Secret{Path: "features.admin", Variable: "APP_ADMIN", Key: "ADMIN_KEY", Text: "the password"}
	byVariable := Secret{Path: "uses.model.m.features.credential", Variable: "OPENAI_API_KEY", Text: "the key"}
	referenced := Secret{Path: "components.api.features.owner", Variable: "OWNER", Text: "the owner", Referenced: true}
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
	} {
		if got := row.secret.Form(row.target); got != row.form {
			t.Errorf("%s: Form is %+v, want %+v", row.scenario, got, row.form)
		}
		if got := row.secret.Declaration(row.target); !reflect.DeepEqual(got, row.declaration) {
			t.Errorf("%s: Declaration is %+v, want %+v", row.scenario, got, row.declaration)
		}
	}
}
