// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// wiringTestFormat is platformTestFormat with a Reach and a Wire of the test's
// own: a node whose data carries server names the part it reaches by that
// string, a server of "" names none, and a node whose data carries serverStarts
// starts a server of its own; a node reaching a part by name has the alias
// written in place of its server where the two differ, its url written empty,
// and the value at its data.secretValues.token, where it holds one, written as
// the name TOKEN_VARIABLE; every other part is reported as reached other than
// through its connection.
func wiringTestFormat() Format {
	f := platformTestFormat()
	f.Platform.Reach = testReach
	f.Platform.Wire = testWire
	return f
}

// testNode reads the data of the node whose id is id, and its position.
func testNode(tree json.RawMessage, id string) (int, map[string]json.RawMessage) {
	var top struct {
		Nodes []struct {
			ID   string                     `json:"id"`
			Data map[string]json.RawMessage `json:"data"`
		} `json:"nodes"`
	}
	_ = json.Unmarshal(tree, &top)
	for i, n := range top.Nodes {
		if n.ID == id {
			return i, n.Data
		}
	}
	return -1, nil
}

func testReach(tree json.RawMessage, node Node) Reach {
	_, data := testNode(tree, node.ID)
	raw, ok := data["server"]
	if !ok {
		return Reach{}
	}
	var server string
	_ = json.Unmarshal(raw, &server)
	switch {
	case server == "":
		return Reach{Refused: "it names no server"}
	case data["serverStarts"] != nil:
		return Reach{Refused: "it starts a server of its own"}
	}
	return Reach{Name: server}
}

func testWire(tree json.RawMessage, part Part) Wiring {
	i, data := testNode(tree, part.Node.ID)
	at := func(tokens ...string) string {
		return Pointer(append([]string{"nodes", strconv.Itoa(i), "data"}, tokens...)...)
	}
	if part.Name == "" {
		return Wiring{Literal: "the test's model is reached as the flow writes it"}
	}
	var w Wiring
	if part.Alias != part.Name {
		name, _ := json.Marshal(part.Alias)
		w.Edits = append(w.Edits, Edit{Pointer: at("server"), Value: name})
	}
	if data["url"] != nil {
		w.Edits = append(w.Edits, Edit{Pointer: at("url"), Value: json.RawMessage(`""`)})
	}
	var secrets map[string]json.RawMessage
	if json.Unmarshal(data["secretValues"], &secrets) == nil && secrets["token"] != nil {
		w.Edits = append(w.Edits, Edit{Pointer: at("secretValues", "token"), Value: json.RawMessage(`"TOKEN_VARIABLE"`)})
	}
	w.Reads = "it reaches the server by name"
	w.LeftForRelease = []string{"the address of " + part.Key}
	return w
}

// wiringSource is a flow of the test's format: a mapped model, which no Reach
// names; two finders naming the server docs, the first holding a planted token
// and both holding an address; a finder naming "My Server", which an alias cannot
// be; one naming my_server, whose variable name collides with that alias; one
// naming no server; and one starting a server of its own.
const wiringSource = `{"id":"flow-w","nodes":[` +
	`{"id":"m1","type":"generic","data":{"type":"OpenAIModel"}},` +
	`{"id":"f1","type":"generic","data":{"type":"Finder","server":"docs","url":"http://planted-address.example/a","secretValues":{"token":"sk-plantedWIREtoken0123456789"}}},` +
	`{"id":"f2","type":"generic","data":{"type":"Finder","server":"docs","url":"http://planted-address.example/b"}},` +
	`{"id":"f3","type":"generic","data":{"type":"Finder","server":"My Server","url":"http://planted-address.example/c"}},` +
	`{"id":"f4","type":"generic","data":{"type":"Finder","server":"my_server"}},` +
	`{"id":"f5","type":"generic","data":{"type":"Finder","server":""}},` +
	`{"id":"f6","type":"generic","data":{"type":"Finder","server":"local","serverStarts":true}}],` +
	`"edges":[]}` + "\n"

// TestANodeNamingAPartIsReachedThroughOneConnectionByName drafts wiringSource
// under the default layout and reads what a format's Reach and Wire make of it:
//
//   - the two finders naming docs reach one part, drafted once and wired once
//     under the alias docs, the second's decision naming the first;
//   - "My Server" cannot be an alias, so its part is wired as my-server and its
//     server is written my-server in the carried flow, which its decision says;
//     my_server would be delivered under the variable name my-server is, so it is
//     wired as my-server-2 and renamed so;
//   - the finder naming no server and the one starting a server of its own stay
//     in the flow, chose nothing, and are reported;
//   - the carried flow is the source with exactly the values Wire wrote — the
//     addresses empty, the two servers renamed, the token named — and the token
//     is not reported as emptied, since what is written there is Wire's;
//   - each wired node's Reads, and the model's Literal, is reported and carried
//     by its decision, and what each part leaves for the Release joins the
//     comment above the platform.
//
// 🔴 PLANTED DEFECTS, each in importPlatform or aliasFor (platform.go):
//
//   - replace `edits = append(edits, wired[i].wiring.Edits...)` with
//     `_ = wired[i].wiring.Edits`: the carried flow keeps the planted addresses and
//     the planted token, and the definition row fails;
//   - in aliasFor, replace `if alias == "" || !aliasShape.MatchString(alias) ||
//     taken[VariableName(alias)] {` with `if alias == "" {`: "My Server" and
//     my_server are written as aliases, and the uses row fails;
//   - delete the block opening `if shared, ok := inst.named[namedPart{socket,
//     reach.Name}]; ok {` together with the `if reach.Name != "" {` around it:
//     f2 draws a second connection, docs-2, and the uses row fails.
func TestANodeNamingAPartIsReachedThroughOneConnectionByName(t *testing.T) {
	t.Parallel()
	out := platformImport(t, wiringTestFormat(), platformCatalog(), "", oneSource(wiringSource))
	agg := out.Files[len(out.Files)-1].Content

	t.Run("uses", func(t *testing.T) {
		const uses = "      uses:\n" +
			"        model:\n          m1:\n            target: m1\n" +
			"        tools:\n" +
			"          docs:\n            target: docs\n" +
			"          my-server:\n            target: my-server\n" +
			"          my-server-2:\n            target: my-server-2\n"
		if !strings.Contains(agg, uses) {
			t.Errorf("the platform does not draw\n%s\nit reads\n%s", uses, agg)
		}
		for _, key := range []string{"docs", "my-server", "my-server-2", "m1"} {
			if !strings.Contains(agg, "\n    "+key+":\n      type: ") {
				t.Errorf("the part %s is not drafted:\n%s", key, agg)
			}
		}
		if strings.Contains(agg, "docs-2") || strings.Contains(agg, "    f5:") || strings.Contains(agg, "    f6:") {
			t.Errorf("a node naming docs again, or one refused, became a part:\n%s", agg)
		}
	})

	t.Run("definition", func(t *testing.T) {
		want := strings.NewReplacer(
			`"http://planted-address.example/a"`, `""`,
			`"http://planted-address.example/b"`, `""`,
			`"http://planted-address.example/c"`, `""`,
			`"server":"My Server"`, `"server":"my-server"`,
			`"server":"my_server"`, `"server":"my-server-2"`,
			`"sk-plantedWIREtoken0123456789"`, `"TOKEN_VARIABLE"`,
		).Replace(strings.TrimSuffix(wiringSource, "\n"))
		if !strings.Contains(agg, "        flow_definition: |\n          "+want+"\n") {
			t.Errorf("the carried flow is not the source with exactly Wire's values written:\nwant %s\nin\n%s", want, agg)
		}
		if strings.Contains(agg, "planted") {
			t.Errorf("a planted value reaches the Aggregate:\n%s", agg)
		}
	})

	t.Run("decisions", func(t *testing.T) {
		for ref, want := range map[string]string{
			"f2": "; wired to the tools socket of core/ai-agent/testplat as docs, through the connection docs, which node f1 " +
				"reaches too; the flow reaches it through that connection: it reaches the server by name",
			"f3": "; wired to the tools socket of core/ai-agent/testplat as my-server; the flow names it \"My Server\", which " +
				"cannot name its connection, so it is named my-server in the flow the draft carries; the flow reaches it " +
				"through that connection: it reaches the server by name",
			"f4": "; wired to the tools socket of core/ai-agent/testplat as my-server-2; the flow names it \"my_server\", which " +
				"cannot name its connection, so it is named my-server-2 in the flow the draft carries; the flow reaches it " +
				"through that connection: it reaches the server by name",
			"m1": "; wired to the model socket of core/ai-agent/testplat as m1; the flow reaches it other than through that " +
				"connection: the test's model is reached as the flow writes it",
		} {
			if d := decisionOf(t, out, ref); d.Chosen == nil || !strings.HasSuffix(d.Rationale, want) {
				t.Errorf("%s was decided %+v: %q, want a choice and a rationale ending %q", ref, d.Chosen, d.Rationale, want)
			}
		}
		for ref, reason := range map[string]string{"f5": "it names no server", "f6": "it starts a server of its own"} {
			want := "kept in the flow core/ai-agent/testplat carries, with no Component of its own: " + reason
			if d := decisionOf(t, out, ref); d.Chosen != nil || d.Rationale != want {
				t.Errorf("%s was decided %+v: %q, want no choice and %q", ref, d.Chosen, d.Rationale, want)
			}
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		var got []string
		for _, d := range out.Diagnostics {
			got = append(got, d.Code+" "+d.SourceRef)
		}
		want := []string{"part-not-placed f5", "part-not-placed f6", "part-address-in-flow m1",
			"part-reads-connection f1", "part-reads-connection f2", "part-reads-connection f3",
			"part-reads-connection f4"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the import raised %v, want %v", got, want)
		}
		if msg := out.Diagnostics[4].Message; msg != "node f2 reaches docs through the tools connection docs of "+
			"core/ai-agent/testplat: it reaches the server by name" {
			t.Errorf("f2's connection is reported as %q", msg)
		}
		if msg := out.Diagnostics[2].Message; msg != "node m1 reaches m1, wired to the model socket of "+
			"core/ai-agent/testplat as m1, other than through that connection: the test's model is reached as the flow "+
			"writes it" {
			t.Errorf("m1's address is reported as %q", msg)
		}
	})

	t.Run("left for the Release", func(t *testing.T) {
		const left = "    #   - the admin slot, the administrator's password\n" +
			"    #   - the address of docs\n" +
			"    #   - the address of my-server\n" +
			"    #   - the address of my-server-2\n" +
			"    #   - the credential each connection under uses presents to the part it reaches, attached on that connection\n"
		if !strings.Contains(agg, left) {
			t.Errorf("the comment above the platform does not read\n%s\nit reads\n%s", left, agg)
		}
	})
}

// TestANodeAConnectionReachesAsWrittenIsWrittenAsItsWiring drafts wiringSource
// with a header planted on f5, a node naming no server, which stays in the flow,
// and on f6, which starts a server of its own, under a format whose Reached says
// a node reaches a connection the draft wires as it is written — and has its
// headers written empty — where it names no server and a part is wired to the
// tools socket, and returns edits with no Reads for every other node:
//
//   - f5's headers are written {} in the carried flow, whatever the
//     empty_credentials setting says, since that is how the node is written for
//     the connection reaching it; its decision and a part-reads-connection
//     warning say so, and its header, which the format locates as a value it
//     marks as none, is neither carried nor emptied as one;
//   - f6's edits, with no Reads, are not written, so its header is carried as
//     written and reported under credential-carried.
//
// 🔴 PLANTED DEFECTS:
//
//   - in carryCredentials (credentials.go), replace
//     `case editWrites(editTokens, c.Pointer):` with `case false:`: f5's header,
//     written by the edit, is also reported as carried, and the diagnostics row
//     fails;
//   - in importPlatform (platform.go), replace `if w.Reads == "" {` with
//     `if false {`: f6's edits are written, and the definition row fails.
func TestANodeAConnectionReachesAsWrittenIsWrittenAsItsWiring(t *testing.T) {
	t.Parallel()
	const (
		reachedKey = "sk-plantedREACHEDheader0123456789"
		keptKey    = "sk-plantedKEPTheader0123456789ab"
	)
	source := strings.Replace(wiringSource, `"server":""}`, `"server":"","headers":{"X-Key":"`+reachedKey+`"}}`, 1)
	source = strings.Replace(source, `"serverStarts":true}`, `"serverStarts":true,"headers":{"X-Key":"`+keptKey+`"}}`, 1)
	format := wiringTestFormat()
	var asked []string
	format.Platform.Reached = func(tree json.RawMessage, node Node, wired []Part) Wiring {
		asked = append(asked, node.ID)
		i, data := testNode(tree, node.ID)
		edit := Edit{Pointer: Pointer("nodes", strconv.Itoa(i), "data", "headers"), Value: json.RawMessage(`{}`)}
		toTools := false
		for _, p := range wired {
			toTools = toTools || p.Socket == "tools"
		}
		var server string
		if json.Unmarshal(data["server"], &server) == nil && server == "" && toTools && data["headers"] != nil {
			return Wiring{Edits: []Edit{edit}, Reads: "its headers are written empty"}
		}
		if data["headers"] != nil {
			return Wiring{Edits: []Edit{edit}}
		}
		return Wiring{}
	}
	for _, settings := range []string{"", `{"empty_credentials":"all"}`} {
		t.Run("settings "+settings, func(t *testing.T) {
			asked = nil
			out := platformImport(t, format, platformCatalog(), settings, oneSource(source))
			agg := out.Files[len(out.Files)-1].Content
			if want := []string{"f5", "f6"}; !reflect.DeepEqual(asked, want) {
				t.Errorf("Reached was asked about %v, want %v", asked, want)
			}
			kept := `"headers":{"X-Key":"` + keptKey + `"}`
			if settings != "" {
				kept = `"headers":{"X-Key":""}`
			}
			if !strings.Contains(agg, `"server":"","headers":{}}`) || !strings.Contains(agg, kept) {
				t.Errorf("f5's headers are not written empty, or f6's not as the setting asks:\n%s", agg)
			}
			var got []string
			for _, d := range out.Diagnostics {
				got = append(got, d.Code+" "+d.SourceRef)
			}
			f6 := "credential-carried f6"
			if settings != "" {
				f6 = "credential-emptied f6"
			}
			want := []string{f6, "part-not-placed f5", "part-not-placed f6", "part-address-in-flow m1",
				"part-reads-connection f1", "part-reads-connection f2", "part-reads-connection f3",
				"part-reads-connection f4", "part-reads-connection f5"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("the import raised %v, want %v", got, want)
			}
			if msg := out.Diagnostics[len(out.Diagnostics)-1].Message; msg != "node f5 is wired to no socket of "+
				"core/ai-agent/testplat, and a connection the draft wires reaches it as it is written: its headers are "+
				"written empty" {
				t.Errorf("f5's wiring is reported as %q", msg)
			}
			if d := decisionOf(t, out, "f5"); !strings.HasSuffix(d.Rationale,
				"; a connection the draft wires reaches it as it is written: its headers are written empty") {
				t.Errorf("f5's decision does not say how a connection reaches it: %q", d.Rationale)
			}
		})
	}
}

// TestAValueTheSettingCarriesNeverStopsAFlowBeingCarried hands the platform a
// credential locator returning a value its pointer does not reach in the
// source's bytes. Under the default the value is carried, so it is never located
// in the bytes: the flow is carried and nothing is reported. Under marked it is
// one to write empty, so the flow is not carried, and the import says a
// credential could not be kept out of the draft. A wiring edit the bytes do not
// hold stops the flow under every setting, and the import says what the draft
// writes for its parts could not be written.
func TestAValueTheSettingCarriesNeverStopsAFlowBeingCarried(t *testing.T) {
	t.Parallel()
	const prefix = "the flow is not carried in values.flow_definition: "
	unreached := func(json.RawMessage) []Credential {
		return []Credential{{Node: "model-1", Field: "gone", Pointer: Pointer("nodes", "0", "data", "gone")}}
	}
	for _, tc := range []struct {
		name, settings string
		edit           bool
		want           []aggrega.Diagnostic
	}{
		{name: "carried by default"},
		{name: "written empty at marked", settings: `{"empty_credentials":"marked"}`,
			want: []aggrega.Diagnostic{{Severity: "warn", Code: "definition-not-carried",
				Message: prefix + "the field gone of node model-1 is not in the source's bytes where its tree holds it, so a " +
					"credential it holds could not be kept out of the draft"}}},
		{name: "a wiring edit", edit: true,
			want: []aggrega.Diagnostic{{Severity: "warn", Code: "definition-not-carried",
				Message: prefix + "the value at /nodes/0/data/gone is not in the source's bytes where its tree holds it, so " +
					"what the draft writes for its parts could not be written"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			format := platformTestFormat()
			format.Platform.Credentials = unreached
			if tc.edit {
				format.Platform.Credentials = nil
				format.Platform.Wire = func(json.RawMessage, Part) Wiring {
					return Wiring{Edits: []Edit{{Pointer: Pointer("nodes", "0", "data", "gone"), Value: json.RawMessage(`""`)}}}
				}
			}
			out := platformImport(t, format, platformCatalog(), tc.settings, oneSource(platformSource))
			if !reflect.DeepEqual(out.Diagnostics, tc.want) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, tc.want)
			}
			carried := strings.Contains(out.Files[len(out.Files)-1].Content, "flow_definition: |")
			if carried != (tc.want == nil) {
				t.Errorf("the flow is carried: %v, want %v", carried, tc.want == nil)
			}
		})
	}
}

// TestNothingReachesAPartThroughAFlowThatIsNotCarried drafts wiringSource with a
// line separator added, which keeps the flow from being carried: the parts are
// wired as before and what each leaves for the Release is still named, but no
// Reads or Literal is reported or carried by a decision, since nothing reaches a
// part through a flow the draft does not carry.
func TestNothingReachesAPartThroughAFlowThatIsNotCarried(t *testing.T) {
	t.Parallel()
	source := strings.Replace(wiringSource, `"type":"OpenAIModel"`, `"type":"OpenAIModel","note":"`+" "+`"`, 1)
	out := platformImport(t, wiringTestFormat(), platformCatalog(), "", oneSource(source))
	agg := out.Files[len(out.Files)-1].Content
	if !strings.Contains(agg, "          docs:\n            target: docs\n") || !strings.Contains(agg, "#   - the address of docs\n") {
		t.Errorf("the parts are not wired, or what they leave for the Release is not named:\n%s", agg)
	}
	for _, d := range out.Diagnostics {
		if d.Code == "part-reads-connection" || d.Code == "part-address-in-flow" {
			t.Errorf("the import reports how a part is reached through a flow it does not carry: %+v", d)
		}
	}
	for _, d := range out.Decisions {
		if strings.Contains(d.Rationale, "through that connection") {
			t.Errorf("%s says how it is reached through a flow the draft does not carry: %q", d.SourceNode.ID, d.Rationale)
		}
	}
}

// TestAFlowWrittenWithCarriageReturnsIsCarriedWithLineFeeds imports a source
// written with CR LF line breaks, and one with a lone carriage return between
// two tokens: each is carried with every carriage return written as one line
// feed — the source otherwise byte for byte, which a JSON reader reads as the
// same value — and one warning names how many it wrote.
//
// 🔴 PLANTED DEFECT: in carry (platform.go), replace
// `text, returns := lineFeedsFor(string(scrubbed))` with
// `text, returns := string(scrubbed), 0`: both rows fail, the flow not carried
// and the carriage returns reported as a character a block cannot carry.
func TestAFlowWrittenWithCarriageReturnsIsCarriedWithLineFeeds(t *testing.T) {
	t.Parallel()
	lf := `{"id":"flow-9",
"nodes":[{"id":"model-1","type":"generic","data":{"type":"OpenAIModel"}}],
"edges":[]}
`
	for _, tc := range []struct {
		name, source string
		returns      int
	}{
		{name: "CR LF line breaks", source: strings.ReplaceAll(lf, "\n", "\r\n"), returns: 3},
		{name: "a lone carriage return", source: strings.Replace(lf, ",\n\"edges\"", ",\r\"edges\"", 1), returns: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := platformImport(t, platformTestFormat(), platformCatalog(), "", oneSource(tc.source))
			want := []aggrega.Diagnostic{{Severity: "warn", Code: "carriage-returns-rewritten",
				Message: "the flow holds " + strconv.Itoa(tc.returns) + " carriage return(s), each written as a line feed " +
					"in values.flow_definition, since a YAML literal block cannot carry one: a JSON reader reads the " +
					"carried flow as the same value"}}
			if !reflect.DeepEqual(out.Diagnostics, want) {
				t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
			}
			agg := out.Files[len(out.Files)-1].Content
			if want := "        flow_definition: " + literalBlock(lf, 8); !strings.Contains(agg, want) {
				t.Errorf("the flow is not carried as the source with line feeds:\nwant %q\nin\n%s", want, agg)
			}
			var a, b any
			if json.Unmarshal([]byte(tc.source), &a) != nil || json.Unmarshal([]byte(lf), &b) != nil || !reflect.DeepEqual(a, b) {
				t.Error("the source and the carried text do not read as the same JSON value")
			}
		})
	}
}

// TestACredentialCarryingAValueIsWrittenAsThatValue locates, through the
// platform layout, a member of a server's configuration with a Value — the value
// with only the credentials inside it emptied — one with only a Detail, and one
// whose value already reads as its Value. With empty_credentials at marked, the
// first is written as its Value, the second as "", and each warning names the
// node and the field with what was emptied, never a value; the third is neither
// written nor reported. Under the default, the first two are carried as written
// and each warning names what of the value its setting would write empty; the
// third, holding nothing to carry, is not reported.
func TestACredentialCarryingAValueIsWrittenAsThatValue(t *testing.T) {
	t.Parallel()
	const source = `{"id":"flow-c","nodes":[{"id":"n1","type":"generic","data":{"type":"ChatOutput",` +
		`"config":"{\"url\":\"http://planted.example\",\"note\":\"kept\"}","loose":"{url: planted}","done":"{}"}}],"edges":[]}` + "\n"
	format := platformTestFormat()
	format.Platform.Credentials = func(json.RawMessage) []Credential {
		return []Credential{
			{Node: "n1", Field: "config", Pointer: Pointer("nodes", "0", "data", "config"), Class: CredentialServerMember,
				Value: json.RawMessage(`"{\"url\":\"\",\"note\":\"kept\"}"`), Detail: "server address"},
			{Node: "n1", Field: "loose", Pointer: Pointer("nodes", "0", "data", "loose"), Class: CredentialServerMember,
				Detail: "it does not read as a JSON object"},
			{Node: "n1", Field: "done", Pointer: Pointer("nodes", "0", "data", "done"), Class: CredentialServerMember,
				Value: json.RawMessage(`"{}"`)},
		}
	}
	const tail = " in the flow the draft carries"
	t.Run("marked", func(t *testing.T) {
		out := platformImport(t, format, platformCatalog(), `{"empty_credentials":"marked"}`, oneSource(source))
		want := []aggrega.Diagnostic{
			{Severity: "warn", Code: "credential-emptied", SourceRef: "n1",
				Message: "config of node n1 is written with its server address empty" + tail},
			{Severity: "warn", Code: "credential-emptied", SourceRef: "n1",
				Message: "the whole value of loose of node n1 is written empty in the flow the draft carries, since it " +
					"does not read as a JSON object"},
		}
		if !reflect.DeepEqual(out.Diagnostics, want) {
			t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
		}
		agg := out.Files[len(out.Files)-1].Content
		if strings.Contains(agg, "planted") || !strings.Contains(agg, `"config":"{\"url\":\"\",\"note\":\"kept\"}","loose":"","done":"{}"`) {
			t.Errorf("the credentials are not written as located:\n%s", agg)
		}
	})
	t.Run("the default", func(t *testing.T) {
		out := platformImport(t, format, platformCatalog(), "", oneSource(source))
		want := []aggrega.Diagnostic{
			{Severity: "warn", Code: "credential-carried", SourceRef: "n1",
				Message: "config of node n1 is carried with its server address as written" + tail + ": " + serverWhy +
					", and setting empty_credentials to marked or all writes its server address empty"},
			{Severity: "warn", Code: "credential-carried", SourceRef: "n1",
				Message: "the value of loose of node n1 is carried as written" + tail + ": " + serverWhy +
					", and setting empty_credentials to marked or all writes it empty"},
		}
		if !reflect.DeepEqual(out.Diagnostics, want) {
			t.Errorf("the import raised %+v, want %+v", out.Diagnostics, want)
		}
		if agg := out.Files[len(out.Files)-1].Content; !strings.Contains(agg, strings.TrimSuffix(source, "\n")) {
			t.Errorf("the flow is not carried as written:\n%s", agg)
		}
	})
}

// refuseSource is a flow of the test's format holding two finders: one naming the
// server docs, and one naming none.
const refuseSource = `{"id":"flow-r","nodes":[` +
	`{"id":"finder","type":"generic","data":{"type":"Finder"}},` +
	`{"id":"f1","type":"generic","data":{"type":"Finder","server":"docs"}}],` +
	`"edges":[]}` + "\n"

// testRefusal is why the test's format refuses a finder naming no server the
// tools socket.
const testRefusal = "the test's tools are reached only through a node naming a server"

// TestASocketTheFormatRefusesANodeIsPassedOver hands the platform a Refuse that
// refuses the tools socket to a node naming no server, and drafts refuseSource
// against a platform whose tools socket is followed by a store socket admitting
// any tool, and against one declaring no such socket. Both finders' Component
// satisfies the tools socket's selector:
//
//   - the finder naming docs is wired to the tools socket in both, the refusal
//     being the node's and not the Component's;
//   - the finder naming none passes the tools socket over: with the store socket
//     declared it is wired there, and its decision says which socket it passed
//     over and why; with none it stays in the flow, chooses nothing, and is
//     reported with the refusal;
//   - Refuse is asked only about the sockets whose selector a part's Component
//     satisfies, in the order the platform declares them, and no further than
//     the socket the part is wired to.
//
// 🔴 PLANTED DEFECTS, each in platform.go:
//
//   - in placePart, replace `if why := refuse(s.Name); why != "" {` with
//     `if why := refuse(s.Name); why == "\x00" {`: both rows fail, the finder
//     naming no server wired to the tools socket;
//   - in importPlatform, delete the line `decision.Rationale +=
//     passedOverClause(passed)` that follows the clause naming a renamed server:
//     the store row fails, its decision not saying the tools socket was passed
//     over.
func TestASocketTheFormatRefusesANodeIsPassedOver(t *testing.T) {
	t.Parallel()
	store := aggrega.CatalogSocket{Name: "store", Selector: "component.aggrega.ai/category=ai-tool"}
	for _, tc := range []struct {
		name    string
		sockets []aggrega.CatalogSocket
		uses    string
		asked   map[string][]string
	}{
		{
			name:    "a later socket takes it",
			sockets: append(append([]aggrega.CatalogSocket{}, testPlatformEntry.Sockets...), store),
			uses: "      uses:\n        tools:\n          docs:\n            target: docs\n" +
				"        store:\n          finder:\n            target: finder\n",
			asked: map[string][]string{"finder": {"tools", "store"}, "f1": {"tools"}},
		},
		{
			name:    "no other socket takes it",
			sockets: testPlatformEntry.Sockets,
			uses:    "      uses:\n        tools:\n          docs:\n            target: docs\n",
			asked:   map[string][]string{"finder": {"tools"}, "f1": {"tools"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			asked := map[string][]string{}
			format := wiringTestFormat()
			format.Platform.Refuse = func(tree json.RawMessage, node Node, socket string) string {
				asked[node.ID] = append(asked[node.ID], socket)
				if _, data := testNode(tree, node.ID); socket == "tools" && data["server"] == nil {
					return testRefusal
				}
				return ""
			}
			out := platformImport(t, format, platformCatalog(tc.sockets...), "", oneSource(refuseSource))
			agg := out.Files[len(out.Files)-1].Content
			if !strings.Contains(agg, tc.uses) {
				t.Errorf("the platform does not draw\n%s\nit reads\n%s", tc.uses, agg)
			}
			if !reflect.DeepEqual(asked, tc.asked) {
				t.Errorf("Refuse was asked about %v, want %v", asked, tc.asked)
			}
			if d := decisionOf(t, out, "f1"); d.Chosen == nil {
				t.Errorf("the finder naming docs was not wired: %q", d.Rationale)
			}

			d := decisionOf(t, out, "finder")
			if tc.name == "a later socket takes it" {
				want := "; wired to the store socket of core/ai-agent/testplat as finder; not to the tools socket, whose " +
					"selector its Component satisfies too, since " + testRefusal + "; "
				if d.Chosen == nil || !strings.Contains(d.Rationale, want) {
					t.Errorf("the finder was decided %+v: %q, want a choice and a rationale holding %q", d.Chosen, d.Rationale,
						want)
				}
				for _, diag := range out.Diagnostics {
					if diag.Code == "part-not-placed" {
						t.Errorf("a part was reported as wired to no socket: %+v", diag)
					}
				}
				return
			}
			reason := "it maps to core/ai-tool/retrieval, which satisfies the selector of the tools socket of " +
				"core/ai-agent/testplat, but " + testRefusal + "; no other socket of core/ai-agent/testplat takes it"
			if d.Chosen != nil || d.Rationale != "kept in the flow core/ai-agent/testplat carries, with no Component of its own: "+reason {
				t.Errorf("the finder was decided %+v: %q, want no choice and the refusal", d.Chosen, d.Rationale)
			}
			want := aggrega.Diagnostic{Severity: "warn", Code: "part-not-placed", SourceRef: "finder",
				Message: "node finder (Finder) is wired to no socket of core/ai-agent/testplat: " + reason + "; it stays in the flow"}
			var placed []aggrega.Diagnostic
			for _, diag := range out.Diagnostics {
				if diag.Code == "part-not-placed" {
					placed = append(placed, diag)
				}
			}
			if !reflect.DeepEqual(placed, []aggrega.Diagnostic{want}) {
				t.Errorf("the parts reported as wired to no socket are %+v, want %+v", placed, want)
			}
			if strings.Contains(agg, "\n    finder:\n") || strings.Contains(agg, "\n          finder:\n") {
				t.Errorf("the refused finder was drafted as a part, or a connection reaches it:\n%s", agg)
			}
		})
	}
}

// singleSource is a flow of the test's format holding three models in this
// order: a widget the settings map to an embedding model, whose data says it
// reads what the platform delivers; a generative model; and a second widget,
// holding an address of its own.
const singleSource = `{"id":"flow-s","nodes":[` +
	`{"id":"e1","type":"generic","data":{"type":"Widget","readsVariables":true}},` +
	`{"id":"m1","type":"generic","data":{"type":"OpenAIModel"}},` +
	`{"id":"e2","type":"generic","data":{"type":"Widget"}}],` +
	`"edges":[]}` + "\n"

// singleSettings maps the test's widget to an embedding model.
const singleSettings = `{"mapping_overrides":{"testflow":{"Widget":{"component":"core/ai-model/openai-embedding","category":"ai-model"}}}}`

// TestASocketDrawingASingleConnectionIsHeldByOnePart drafts singleSource against
// a platform whose model socket admits any model, so all three models are placed
// in it, and declares that socket as drawing a single connection:
//
//   - preferring a generative model, the connection goes to m1, the one part
//     whose Component carries that label, though it is not the first; preferring
//     a label no part's Component carries, it goes to the first part, e1;
//   - every other model stays in the flow: it chooses nothing, is not drafted,
//     and is reported, in the order of the nodes, with the part holding the
//     connection, why it holds it, and how the flow reaches it as its node is
//     written, as Standing says — asked with the node in that socket under the
//     holder's key and alias;
//   - the holder's decision says it holds the socket's single connection;
//   - with no socket declared as drawing a single connection, all three are
//     wired, as before.
//
// 🔴 PLANTED DEFECTS, each in platform.go:
//
//   - in holdSingleSockets, replace
//     `if carriesEvery(labels[placements[i].target], s.Prefer) {` with
//     `if false {`: the generative row fails, e1 holding the connection;
//   - in importPlatform, replace `if pl.heldBy != "" {` with
//     `if pl.heldBy == "\x00" {`: both rows declaring the socket fail, the
//     platform drawing three model connections.
func TestASocketDrawingASingleConnectionIsHeldByOnePart(t *testing.T) {
	t.Parallel()
	anyModel := aggrega.CatalogSocket{Name: "model", Selector: "component.aggrega.ai/category=ai-model"}
	const generative = "model.aggrega.ai/capability"
	for _, tc := range []struct {
		name   string
		single []SingleSocket
		holder string
		why    string
		held   []string
	}{
		{name: "a generative model preferred", single: []SingleSocket{{Name: "model", Prefer: map[string]string{generative: "generative"}}},
			holder: "m1", why: "its Component carries model.aggrega.ai/capability: generative", held: []string{"e1", "e2"}},
		{name: "a label no part carries preferred", single: []SingleSocket{{Name: "model", Prefer: map[string]string{generative: "reasoning"}}},
			holder: "e1", why: "it is the first of the parts placed there in the flow, none of whose Components carries " +
				"model.aggrega.ai/capability: reasoning", held: []string{"m1", "e2"}},
		{name: "no socket drawing a single connection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var asked []Part
			format := platformTestFormat()
			format.Platform.Single = tc.single
			format.Platform.Standing = func(tree json.RawMessage, part Part) (bool, string) {
				asked = append(asked, part)
				if _, data := testNode(tree, part.Node.ID); data["readsVariables"] != nil {
					return true, "it reads what the platform delivers"
				}
				return false, "it holds an address of its own"
			}
			out := platformImport(t, format, platformCatalog(anyModel), singleSettings, oneSource(singleSource))
			agg := out.Files[len(out.Files)-1].Content

			if tc.single == nil {
				const uses = "      uses:\n        model:\n          e1:\n            target: e1\n" +
					"          m1:\n            target: m1\n          e2:\n            target: e2\n"
				if !strings.Contains(agg, uses) {
					t.Errorf("the platform does not draw\n%s\nit reads\n%s", uses, agg)
				}
				if len(asked) != 0 {
					t.Errorf("Standing was asked about %+v with no socket drawing a single connection", asked)
				}
				return
			}

			uses := "      uses:\n        model:\n          " + tc.holder + ":\n            target: " + tc.holder + "\n"
			if !strings.Contains(agg, uses) || strings.Count(agg, "            target: ") != 1 {
				t.Errorf("the platform does not draw the one connection\n%s\nit reads\n%s", uses, agg)
			}
			holderClause := "; the model socket of core/ai-agent/testplat draws a single connection, which this part holds " +
				"since " + tc.why
			if d := decisionOf(t, out, tc.holder); d.Chosen == nil || !strings.HasSuffix(d.Rationale, holderClause) {
				t.Errorf("the holder %s was decided %+v: %q, want a choice and a rationale ending %q", tc.holder, d.Chosen,
					d.Rationale, holderClause)
			}

			standing := map[string]string{
				"e1": "; as its node is written, the flow reaches it through the connection " + tc.holder +
					": it reads what the platform delivers",
				"m1": "; as its node is written, the flow reaches it other than through the connection " + tc.holder +
					": it holds an address of its own",
				"e2": "; as its node is written, the flow reaches it other than through the connection " + tc.holder +
					": it holds an address of its own",
			}
			targets := map[string]string{"e1": "core/ai-model/openai-embedding", "m1": "core/ai-model/openai",
				"e2": "core/ai-model/openai-embedding"}
			var want []aggrega.Diagnostic
			var wantAsked []Part
			for _, ref := range tc.held {
				reason := "it maps to " + targets[ref] + ", which satisfies the selector of the model socket of " +
					"core/ai-agent/testplat, but that socket draws a single connection, which reaches " + tc.holder +
					", wired from node " + tc.holder + " since " + tc.why + standing[ref]
				if d := decisionOf(t, out, ref); d.Chosen != nil || d.Rationale != "kept in the flow core/ai-agent/testplat "+
					"carries, with no Component of its own: "+reason {
					t.Errorf("%s was decided %+v: %q, want no choice and %q", ref, d.Chosen, d.Rationale, reason)
				}
				if strings.Contains(agg, "\n    "+ref+":\n") {
					t.Errorf("the held-out %s is drafted as a part:\n%s", ref, agg)
				}
				want = append(want, aggrega.Diagnostic{Severity: "warn", Code: "part-not-placed", SourceRef: ref,
					Message: "node " + ref + " (" + map[string]string{"e1": "Widget", "m1": "OpenAIModel", "e2": "Widget"}[ref] +
						") is wired to no socket of core/ai-agent/testplat: " + reason + "; it stays in the flow"})
				node := Node{ID: ref, Type: map[string]string{"e1": "Widget", "m1": "OpenAIModel", "e2": "Widget"}[ref]}
				wantAsked = append(wantAsked, Part{Node: node, Socket: "model", Key: tc.holder, Alias: tc.holder})
			}
			var placed []aggrega.Diagnostic
			for _, d := range out.Diagnostics {
				if d.Code == "part-not-placed" {
					placed = append(placed, d)
				}
			}
			if !reflect.DeepEqual(placed, want) {
				t.Errorf("the import reports\n%+v\nwant\n%+v", placed, want)
			}
			if !reflect.DeepEqual(asked, wantAsked) {
				t.Errorf("Standing was asked about %+v, want %+v", asked, wantAsked)
			}
		})
	}
}

// TestVariableName pins the name a platform delivering a connection under
// variables named for its alias gives the alias.
func TestVariableName(t *testing.T) {
	t.Parallel()
	for alias, want := range map[string]string{"docs": "DOCS", "my-server": "MY_SERVER", "my_server": "MY_SERVER",
		"Kb2": "KB2", "a.b c": "A_B_C"} {
		if got := VariableName(alias); got != want {
			t.Errorf("VariableName(%q) = %q, want %q", alias, got, want)
		}
	}
}
