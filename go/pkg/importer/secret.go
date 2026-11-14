// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import "strings"

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
	// Referenced says the slot takes core/referenced-secret whatever the target,
	// as one the catalog attaches that type on does: named by its Secret on helm,
	// as every secret there is, and elsewhere by the variable reference, which the
	// compose realization of that type delivers as written.
	Referenced bool
	// Entries, where set, make the secret a set of variables rather than one
	// value: each one the consumer reads under its own name, as a server's
	// environment is. Variable and Key are not read for a set.
	Entries []SecretEntry
}

// SecretEntry is one variable of a secret set, as [Secret.Entries] lists them.
type SecretEntry struct {
	// Key is the name the consumer reads the variable under.
	Key string
	// Value is the variable's value as the draft writes it, written as a
	// double-quoted scalar: a variable reference, read from the environment of
	// whoever starts a target substituting one; a value the source holds, which
	// the import's credential decision carried; or empty, a value still owed.
	Value string
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
	secretSetType             = "core/referenced-secret-set"
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
//   - for helm, with core/referenced-secret, named by its secret resource, a
//     Kubernetes Secret, since Kubernetes substitutes no variable reference; a
//     drafted Manifest maps that type to helm/k8s-existing-secret;
//   - for every other target, and for none, by the variable reference
//     ${Variable} as its origin, which a target substituting variable
//     references, as Docker Compose does when a stack starts, reads from the
//     environment of whoever starts it, so no file carries the value: where s is
//     Referenced, with core/referenced-secret, whose compose realization delivers
//     the origin as written, and which the drafted Aggregate does not declare,
//     the slot being the catalog's; otherwise with core/secret-value, which for
//     compose the drafted Aggregate declares with its key;
//   - a set ([Secret.Entries]), for every target but helm, with
//     core/referenced-secret-set, its entries written as their values are, each
//     under its own name, so neither a key nor a declaration is needed; for helm,
//     as every secret there is, by its Secret, which holds one data key per
//     variable.
//
// This is the one place the target decides a secret's form, so a secret a draft
// attaches and a secret it passes down as a module's parameter are written alike.
func (s Secret) Form(target string) SecretForm {
	switch {
	case target == helmTarget:
		return SecretForm{Type: referencedSecretType, ByName: true, Realization: helmReferencedRealization}
	case s.Referenced:
		return SecretForm{Type: referencedSecretType}
	case len(s.Entries) > 0:
		return SecretForm{Type: secretSetType}
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
// declares it through this, so the Aggregate and the Release agree. A set
// declares nothing: each of its entries arrives under its own name.
func (s Secret) Declaration(target string) SecretDeclaration {
	form, live := s.Form(target), true
	if !form.Keyed {
		if target != "" || s.Referenced || len(s.Entries) > 0 {
			return SecretDeclaration{}
		}
		target, live = composeTarget, false
		form = s.Form(target)
	}
	return SecretDeclaration{Lines: []string{s.slot() + ":", "  type: " + form.Type, "  values:", "    key: " + s.key()},
		Target: target, Live: live, Slot: s.slot()}
}

// OwnSlot reports whether s is attached on one of its instance's own slots, its
// path features.<slot>, rather than on one of the instance's connections. It is
// the one reading of which of the two a secret is: DeclarationsText declares only
// the first, and an importer declaring a connection's secret on that connection
// tells the two apart by it.
func (s Secret) OwnSlot() bool {
	return strings.HasPrefix(s.Path, "features.") && strings.Count(s.Path, ".") == 1
}

// DeclarationsText is what a drafted Aggregate declares, for target, of each of
// secrets attached on an instance's own slots — those whose path is
// features.<slot>; a secret on one of its connections is declared there — inside
// the instance keyed key, whose own keys are written at indent: each declaration
// as [SecretDeclaration.Text] writes it, a features key opened once, above the
// first one written live, where inFeatures does not say the instance already opens
// one. Every Aggregate writer declaring an instance's secrets writes them through
// this, so they are written one way.
func DeclarationsText(target, indent, key string, secrets []Secret, inFeatures bool) string {
	var b strings.Builder
	for _, s := range secrets {
		if !s.OwnSlot() {
			continue
		}
		decl := s.Declaration(target)
		if len(decl.Lines) == 0 {
			continue
		}
		if decl.Live && !inFeatures {
			b.WriteString(indent + "features:\n")
			inFeatures = true
		}
		b.WriteString(decl.Text(indent, key, inFeatures))
	}
	return b.String()
}

// Text is d as a drafted Aggregate writes it inside what is keyed key — an
// instance, or one of its connections — beneath a comment saying which target
// needs the lines and why ([DeclarationWhy]); empty where d declares nothing.
// Where d is live the lines are written as declarations; otherwise each is
// commented out in place, so an author building for that target uncomments them
// where they stand. Where inFeatures is set what the lines are written in
// already declares features, so they are written inside that block, without a
// second features key; otherwise a features key opens them. at is the
// indentation of the keys of what they are written in. Every comment line is
// written through [CommentLine], so it reads back as written once the Aggregate
// is evaluated as a template. Every Aggregate writer declaring a secret writes
// the declaration through this, so it is written one way.
func (d SecretDeclaration) Text(at, key string, inFeatures bool) string {
	if len(d.Lines) == 0 {
		return ""
	}
	indent, lines := at+"  ", d.Lines
	if !inFeatures {
		indent, lines = at, []string{"features:"}
		for _, line := range d.Lines {
			lines = append(lines, "  "+line)
		}
	}
	var b strings.Builder
	if d.Live {
		b.WriteString(CommentLine(indent, "For the "+d.Target+" target, the lines below declare "+d.Slot+" on "+
			key+": "+DeclarationWhy+"."))
		for _, line := range lines {
			b.WriteString(indent + line + "\n")
		}
		return b.String()
	}
	b.WriteString(CommentLine(indent, "For the "+d.Target+" target only, uncomment the lines below, which declare "+
		d.Slot+" on "+key+": "+DeclarationWhy+"."))
	for _, line := range lines {
		b.WriteString(CommentLine(indent, line))
	}
	return b.String()
}

// secretNode is the attachment s is written as in a drafted Release for target,
// keyed by its slot, as its form says: its type, and beneath a comment its
// infra.origin — the variable reference, saying no file carries the value, or
// empty, saying it names the secret resource holding the value — or, for a set
// written as its entries, its infra.entries, each key and value as given.
func secretNode(s Secret, target string) *bodyNode {
	form := s.Form(target)
	infra := &bodyNode{text: "infra:"}
	node := &bodyNode{text: s.slot() + ":", children: []*bodyNode{{text: "type: " + form.Type}, infra}}
	if form.Type == secretSetType {
		entries := &bodyNode{text: "entries:"}
		for _, e := range s.Entries {
			entries.children = append(entries.children, &bodyNode{text: "- key: " + QuotedString(e.Key),
				children: []*bodyNode{{text: "value: " + QuotedString(e.Value)}}})
		}
		infra.children = append(commentNodes("Attached by reference wherever the source names a variable: "+s.Text+
			", one entry per variable, each written as the source writes it, so a variable reference stays one, which "+
			"a target substituting variable references, as Docker Compose does when the stack starts, reads from the "+
			"environment, and an empty value is one still owed."), entries)
		return node
	}
	origin, comment := variableReference(s.Variable), "Attached by reference, never as a value: "+s.Text+", "+
		readFromVariable(s.Variable)+"."
	if form.ByName {
		holds := "the name of the secret resource holding " + s.Text + ", as the Manifest's realization of " +
			form.Type + " reads it"
		switch {
		case target == helmTarget && len(s.Entries) > 0:
			holds = "the name of a Kubernetes Secret already in the namespace holding " + s.Text + ", one data key " +
				"per variable, each named as the variable it holds: " + strings.Join(s.entryKeys(), ", ")
		case target == helmTarget:
			holds = "the name of a Kubernetes Secret already in the namespace holding, under the data key value, " +
				s.Text
		}
		origin, comment = `""`, leftHeading+" "+holds+"."
	}
	infra.children = append(commentNodes(comment), &bodyNode{text: "origin: " + origin})
	return node
}

// secretParameterNodes are the lines a drafted Release declares s with under its
// spec.parameters, s a parameter holding a secret ([ReleaseDraft.SecretParameters])
// keyed by its Path, as its form for target says: where the form names the
// secret by its resource, the parameter holding an empty string, a value still
// owed, beneath the comment a left parameter is written with; otherwise the
// parameter holding the variable reference, beneath a comment saying where it is
// read from.
func secretParameterNodes(s Secret, target string) []*bodyNode {
	item := LeftItem{Path: s.Path, Text: s.Text}
	if s.Form(target).ByName {
		return leftParameterNodes(item)
	}
	return append(commentNodes("Passed by reference, never as a value: "+leftLine(item)+"; it is "+
		readFromVariable(s.Variable)+"."), &bodyNode{text: s.Path + ": " + variableReference(s.Variable)})
}

// variableReference is the origin a secret read from variable is written with
// where its form names no secret resource: the variable reference ${variable}, as
// a double-quoted scalar.
func variableReference(variable string) string {
	return QuotedString("${" + variable + "}")
}

// readFromVariable says where a secret written as [variableReference] is read
// from, completing the comment above it.
func readFromVariable(variable string) string {
	return "read from the variable " + variable + ", which a target substituting variable references, as Docker " +
		"Compose does when the stack starts, reads from the environment, so no file carries the value"
}

// entryKeys are the names of the variables a set holds, in its order.
func (s Secret) entryKeys() []string {
	out := make([]string, 0, len(s.Entries))
	for _, e := range s.Entries {
		out = append(out, e.Key)
	}
	return out
}
