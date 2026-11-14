// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

// This file holds the one place a drafted secret's form is decided for a target:
// the feature type it is attached with, how the Release names its origin, whether
// the drafted Aggregate declares its key, and what a drafted Manifest maps its type
// to. The Release writer, the platform draft's Aggregate writer and every importer
// drafting an Aggregate of its own read it through Secret.Form and
// Secret.Declaration, so the two halves of one secret cannot disagree.

// Secret is a secret an entry of a drafted Release attaches, as
// [ReleaseEntry.Secrets] lists them, written for the target the import drafts
// for, as [Secret.Form] decides. It is always a reference, never a value: no value
// a source holds is ever written into a draft.
type Secret struct {
	// Path is the attachment's key path, relative to the entry: features.<slot>
	// for a slot of the instance, uses.<socket>.<alias>.features.<slot> for a slot
	// of one of its connections.
	Path string
	// Variable is the name of the variable the secret is read from where it is
	// written as a variable reference: the source's own name for it, or one named
	// after what it is. It is written as it is, so the caller gives a variable
	// name — letters, digits and underscores, not starting with a digit.
	Variable string
	// Key is the key the secret's value arrives under where the drafted Aggregate
	// declares the attachment with a key of its own ([Secret.Declaration]); empty
	// names Variable. It is written as it is, so the caller gives one a Recipe can
	// read: letters, digits and underscores.
	Key string
	// Text says what the secret is, written into the comment above it. It is
	// written into a comment as it is, so it carries nothing read from a source a
	// comment cannot carry.
	Text string
	// Referenced says the slot takes a secret named by reference whatever the
	// target, as one the catalog attaches core/referenced-secret on does.
	Referenced bool
}

// ConnectionCredential is the secret an entry keyed key attaches where the
// instance presents a credential on the connection its socket draws under alias,
// reaching the instance keyed target: the credential, attached on that
// connection, read from variable — or, where variable is empty, from the variable
// named after target, upper-cased with every character other than a letter or a
// digit an underscore, followed by _CREDENTIAL. Every importer attaching such a
// credential attaches it through this, so it is named one way.
func ConnectionCredential(key, socket, alias, target, variable string) Secret {
	if variable == "" {
		variable = VariableName(target) + "_CREDENTIAL"
	}
	return Secret{Path: "uses." + socket + "." + alias + ".features.credential", Variable: variable,
		Text: "the credential " + key + " presents to " + target + " on its " + socket + " connection"}
}

// The targets whose secrets a draft writes apart from every other: helm, where a
// secret is named by its Kubernetes Secret, since Kubernetes substitutes no
// variable reference; and compose, whose Recipes deliver every secret a component
// is attached into one environment, under the key its attachment names, so each
// is declared with a key of its own.
const (
	helmTarget    = "helm"
	composeTarget = "compose"
)

// The feature types a drafted secret is attached with, and the realization a
// drafted Manifest maps core/referenced-secret to on the helm target: the one
// pointing a workload at a Secret already in its namespace, which renders
// nothing of the credential and is the one a Release can name by its Secret
// alone, while the catalog offers several.
const (
	secretValueType           = "core/secret-value"
	referencedSecretType      = "core/referenced-secret"
	helmReferencedRealization = "helm/k8s-existing-secret"
)

// SecretForm is how a draft attaches one secret for one target, as
// [Secret.Form] decides it.
type SecretForm struct {
	// Type is the feature type the secret is attached with.
	Type string
	// ByName says the Release names the secret resource holding the value, its
	// infra.origin written empty for the Release to supply; otherwise the Release
	// writes its infra.origin as the variable reference ${Variable}.
	ByName bool
	// Keyed says the drafted Aggregate declares the attachment with its key, so
	// the value arrives under a key of its own.
	Keyed bool
	// Realization is what a drafted Manifest maps Type to for the target, empty
	// where the draft maps nothing.
	Realization string
}

// Form is how a draft attaches s for target, target empty where the import names
// none:
//
//   - for helm, and wherever s is Referenced, with core/referenced-secret, named by
//     its secret resource; for helm, a drafted Manifest maps that type to
//     helm/k8s-existing-secret;
//   - for every other target, and for none, with core/secret-value, whose origin
//     is the variable reference ${Variable}, which a target substituting variable
//     references, as Docker Compose does when a stack starts, reads from the
//     environment of whoever starts it, so no file carries the value; for compose
//     the drafted Aggregate declares it with its key.
func (s Secret) Form(target string) SecretForm {
	if s.Referenced || target == helmTarget {
		form := SecretForm{Type: referencedSecretType, ByName: true}
		if target == helmTarget {
			form.Realization = helmReferencedRealization
		}
		return form
	}
	return SecretForm{Type: secretValueType, Keyed: target == composeTarget}
}

// key is the key s is declared with where its form is keyed.
func (s Secret) key() string {
	if s.Key != "" {
		return s.Key
	}
	return s.Variable
}

// slot is the attachment slot s fills, the last segment of its path.
func (s Secret) slot() string {
	return leaf(s.Path)
}

// SecretDeclaration is what a drafted Aggregate declares of a secret, on the
// instance or the connection its path names, as [Secret.Declaration] gives it.
type SecretDeclaration struct {
	// Lines are the attachment's lines, beginning with its slot and indented
	// relative to it, declaring its type and the key its value arrives under;
	// none where the Aggregate declares nothing of the secret.
	Lines []string
	// Target is the target the lines are for.
	Target string
	// Live says the lines are written as lines of the draft; otherwise the draft
	// shows them commented out, for an author building for Target to uncomment.
	Live bool
	// Slot is the attachment slot the lines declare.
	Slot string
}

// DeclarationWhy completes the comment a drafted Aggregate writes above a
// [SecretDeclaration], saying why its target needs it.
const DeclarationWhy = "a secret the Release alone attaches arrives under the key value, so each is declared with " +
	"a key of its own and no two arrive under one; the Release attaches its origin, naming this type or none"

// Declaration is what a drafted Aggregate declares of s for target: for a target
// whose form of s is keyed, the lines, live; where the import names no target and
// s is not Referenced, the compose target's lines, which the draft shows
// commented out; otherwise none. Every Aggregate writer declaring a secret's key
// declares it through this, so the Aggregate and the Release agree.
func (s Secret) Declaration(target string) SecretDeclaration {
	form, live := s.Form(target), true
	if !form.Keyed {
		if target != "" || s.Referenced {
			return SecretDeclaration{}
		}
		target, live = composeTarget, false
		form = s.Form(target)
	}
	return SecretDeclaration{Lines: []string{s.slot() + ":", "  type: " + form.Type, "  values:", "    key: " + s.key()},
		Target: target, Live: live, Slot: s.slot()}
}

// secretNode is the attachment s is written as in a drafted Release for target,
// keyed by its slot, as its form says: its type, and its infra.origin beneath a
// comment — the variable reference, saying no file carries the value, or empty,
// saying it names the secret resource holding the value.
func secretNode(s Secret, target string) *bodyNode {
	form := s.Form(target)
	origin, comment := QuotedString("${"+s.Variable+"}"), "Attached by reference, never as a value: "+s.Text+
		", read from the variable "+s.Variable+", which a target substituting variable references, as Docker "+
		"Compose does when the stack starts, reads from the environment, so no file carries the value."
	if form.ByName {
		holds := "the name of the secret resource holding " + s.Text + ", as the Manifest's realization of " +
			form.Type + " reads it"
		if target == helmTarget {
			holds = "the name of a Kubernetes Secret already in the namespace holding, under the data key value, " +
				s.Text
		}
		origin, comment = `""`, leftHeading+" "+holds+"."
	}
	infra := &bodyNode{text: "infra:"}
	infra.children = append(commentNodes(comment), &bodyNode{text: "origin: " + origin})
	return &bodyNode{text: s.slot() + ":", children: []*bodyNode{{text: "type: " + form.Type}, infra}}
}
