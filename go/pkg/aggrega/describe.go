// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package aggrega

// MarshalDescriptor encodes a plugin's describe() self-report to the wire bytes
// the host reads once at load. A guest constructs a typed [PluginDescriptor] —
// plugin type, exports with their host-function and capability fields,
// capabilities, a provider contract, commands and their flags, metadata — and
// calls MarshalDescriptor to emit the canonical wire form through the ABI
// codec, rather than hand-assembling the JSON.
//
// MarshalDescriptor folds the build-provenance stamp into the descriptor's
// metadata: when [Version] or [Commit] is set (injected at build time via the
// Go linker), the corresponding metadata field is populated, allocating the
// metadata object when the descriptor declared none. An explicit value already
// present on the descriptor's metadata is overwritten by a set build-provenance
// value, so the linker-injected stamp is authoritative. An unstamped build
// leaves metadata untouched, so the descriptor's wire form is byte-identical to
// one built without the provenance fields.
//
// The codec omits every absent optional field: a descriptor that declares no
// contract, no metadata, and no commands marshals without those keys, so a
// migrated guest emits the same bytes it did as a hand-written JSON literal.
func MarshalDescriptor(d PluginDescriptor) ([]byte, error) {
	if Version != "" || Commit != "" {
		meta := PluginDescriptorMeta{}
		if d.Metadata != nil {
			meta = *d.Metadata
		}
		if Version != "" {
			meta.Version = Version
		}
		if Commit != "" {
			meta.Commit = Commit
		}
		d.Metadata = &meta
	}
	return d.MarshalJSON()
}
