// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package importer

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// TestTheConventionsAnImporterDraftingItsOwnSharesAreImportsOwn holds the
// package's exports for an importer that drafts from its sources itself to what
// Import itself does: the same sources read off a request, the same parse-failed
// and no-nodes diagnostics, the same target_namespace reading, the same instance
// keys and the same Aggregate description.
func TestTheConventionsAnImporterDraftingItsOwnSharesAreImportsOwn(t *testing.T) {
	t.Parallel()
	primary := aggrega.ImportInput{Format: "f", Bytes: []byte("x")}
	if got := Sources(primary); !reflect.DeepEqual(got, []aggrega.ImportSource{{Name: "", Bytes: []byte("x")}}) {
		t.Errorf("Sources of a request carrying primary bytes = %+v, want them as one source with no name", got)
	}
	named := aggrega.ImportInput{Format: "f", Bytes: []byte("x"), Sources: []aggrega.ImportSource{{Name: "a", Bytes: []byte("y")}}}
	if got := Sources(named); !reflect.DeepEqual(got, named.Sources) {
		t.Errorf("Sources of a request carrying named sources = %+v, want them", got)
	}

	src := aggrega.ImportSource{Name: "flow.json"}
	if got, want := ParseFailed(src, "f", errors.New("bad")), (aggrega.Diagnostic{Severity: "halt", Code: "parse-failed",
		Message: `could not parse source "flow.json" as f: bad`, SourceRef: "flow.json"}); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFailed = %+v, want %+v", got, want)
	}
	if got, want := NoNodes(src, "f"), (aggrega.Diagnostic{Severity: "warn", Code: "no-nodes",
		Message: `source "flow.json" carried no recognisable f nodes`, SourceRef: "flow.json"}); !reflect.DeepEqual(got, want) {
		t.Errorf("NoNodes = %+v, want %+v", got, want)
	}

	for _, row := range []struct{ settings, want string }{
		{"", "imported"}, {`{}`, "imported"}, {`{"target_namespace":""}`, "imported"},
		{`{"target_namespace":"payments"}`, "payments"}, {`not json`, "imported"},
	} {
		if got := TargetNamespace(json.RawMessage(row.settings)); got != row.want {
			t.Errorf("TargetNamespace(%q) = %q, want %q", row.settings, got, row.want)
		}
	}

	used := map[string]bool{}
	var keys []string
	for _, name := range []string{"GitHub Server", "github-server", "io.github.x/y", "!!!"} {
		keys = append(keys, UniqueKey(used, name))
	}
	if want := []string{"github-server", "github-server-2", "io-github-x-y", "node"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("UniqueKey wrote %q, want %q", keys, want)
	}

	if got := DraftDescription("mcp"); got != "Draft Aggregate imported from a source in the mcp format." {
		t.Errorf("DraftDescription(mcp) = %q", got)
	}
}
