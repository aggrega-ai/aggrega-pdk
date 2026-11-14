// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aggrega-ai/aggrega-pdk/go/pkg/aggrega"
)

// releasingFormat is the test's platform format naming the image it drafts, the
// version a source states — its top-level version — the platform's own secret,
// keyed ADMIN_PASSWORD, and the model socket's credential variable, also
// ADMIN_PASSWORD, so the credential's variable and key must step aside to
// ADMIN_PASSWORD_2.
func releasingFormat(image string) Format {
	f := platformTestFormat()
	f.Platform.LeftForRelease = nil
	f.Platform.Image = image
	f.Platform.SourceVersion = func(tree json.RawMessage) string {
		var top struct {
			Version string `json:"version"`
		}
		_ = json.Unmarshal(tree, &top)
		return top.Version
	}
	f.Platform.Secrets = []Secret{{Path: "features.admin", Variable: "TESTPLAT_ADMIN", Key: "ADMIN_PASSWORD",
		Text: "the administrator's password"}}
	f.Platform.CredentialVariables = map[string]string{"model": "ADMIN_PASSWORD"}
	return f
}

// releasingSource is platformSource stating version 1.0.0 of its platform.
var releasingSource = strings.Replace(platformSource, `{"id":"flow-7",`, `{"id":"flow-7","version":"1.0.0",`, 1)

// TestThePlatformDraftsItsImageAndItsSecretsForTheTarget imports releasingSource
// under releasingFormat for compose, for no target and for helm, and reads the
// files drafted:
//
//   - the platform's entry sets infra.image to the image the format names, quoted,
//     beneath the comment saying it is the version the importer drafts, naming
//     the source's version 1.0.0 where the image's tag differs, and not where it
//     is the same;
//   - the platform's own secret and the model connection's credential are attached
//     as references for the target — ${TESTPLAT_ADMIN} and ${ADMIN_PASSWORD_2}
//     for compose and for none, an empty Secret name for helm — while the tools
//     connection, whose socket the format names no variable for, leaves its
//     credential in the comment above the entry;
//   - for compose the Aggregate declares the model connection's credential on its
//     connection with the key ADMIN_PASSWORD_2, live; for no target it shows the
//     lines commented out; for helm it writes none;
//   - for helm alone the draft writes manifest.yaml, mapping
//     core/referenced-secret to helm/k8s-existing-secret.
//
// 🔴 PLANTED DEFECTS:
//   - in credentials (platform.go), drop the loop over the platform's Secrets
//     filling taken: the compose row fails, the key and the variable written
//     ADMIN_PASSWORD, the platform's own secret's key;
//   - in instanceEntry (platform.go), drop the ReleaseValue of infra.image: every
//     row fails.
func TestThePlatformDraftsItsImageAndItsSecretsForTheTarget(t *testing.T) {
	t.Parallel()
	declaration := "            # features:\n" +
		"            #   credential:\n" +
		"            #     type: core/secret-value\n" +
		"            #     values:\n" +
		"            #       key: ADMIN_PASSWORD_2\n"
	live := "          model-1:\n" +
		"            target: model-1\n" +
		"            # For the compose target, the lines below declare credential on model-1: " + DeclarationWhy + ".\n" +
		"            features:\n" +
		"              credential:\n" +
		"                type: core/secret-value\n" +
		"                values:\n" +
		"                  key: ADMIN_PASSWORD_2\n"
	byReference := []string{
		"      features:\n        admin:\n          type: core/secret-value\n",
		"            origin: \"${TESTPLAT_ADMIN}\"\n",
		"                type: core/secret-value\n",
		"                  origin: \"${ADMIN_PASSWORD_2}\"\n",
	}
	byName := []string{
		"      features:\n        admin:\n          type: core/referenced-secret\n",
		"the name of a Kubernetes Secret already in the namespace holding, under the data key value, the " +
			"administrator's password.\n            origin: \"\"\n",
		"          model-1:\n            features:\n              credential:\n                type: core/referenced-secret\n",
	}
	imageComment := "        # The image running core/ai-agent/testplat, at the version this importer drafts, which the " +
		"catalog's Recipes for it are known to run; the import does not raise or lower it to follow the source."
	stated := " The source states version \"1.0.0\" of the platform.\n"
	for _, row := range []struct {
		scenario, target, image string
		release, aggregate      []string
		absent                  []string
		manifest                bool
	}{
		{"compose", "compose", "testplat/server:2.0.0",
			append([]string{imageComment + stated + "        image: \"testplat/server:2.0.0\"\n"}, byReference...),
			[]string{live}, []string{declaration}, false},
		{"no target", "", "testplat/server:2.0.0",
			append([]string{imageComment + stated}, byReference...),
			[]string{"            target: model-1\n" + "            # For the compose target only, uncomment the lines " +
				"below, which declare credential on model-1: " + DeclarationWhy + ".\n" + declaration},
			[]string{"\n                  key: ADMIN_PASSWORD_2\n"}, false},
		{"helm, the source's version", "helm", "testplat/server:1.0.0",
			append([]string{imageComment + "\n        image: \"testplat/server:1.0.0\"\n"}, byName...),
			nil, []string{"ADMIN_PASSWORD_2", "${", "states version"}, true},
	} {
		in := aggrega.ImportInput{Format: testFormatName, Catalog: platformCatalog(), Bytes: []byte(releasingSource),
			Target: row.target}
		out := Import(in, releasingFormat(row.image), (&parseRecorder{}).parse)
		files := map[string]string{}
		for _, f := range out.Files {
			files[f.Path] = f.Content
		}
		release, agg := files[ReleaseFileName], files["aggregate.yaml"]
		for _, want := range row.release {
			if !strings.Contains(release, want) {
				t.Errorf("%s: the Release does not hold\n%s\nit reads\n%s", row.scenario, want, release)
			}
		}
		for _, want := range row.aggregate {
			if !strings.Contains(agg, want) {
				t.Errorf("%s: the Aggregate does not hold\n%s\nit reads\n%s", row.scenario, want, agg)
			}
		}
		for _, absent := range row.absent {
			if strings.Contains(release, absent) || strings.Contains(agg, absent) {
				t.Errorf("%s: the drafts hold %q:\nRelease\n%s\nAggregate\n%s", row.scenario, absent, release, agg)
			}
		}
		if want := "    #   - uses.tools.finder.features.credential, the credential testplat presents to finder on its " +
			"tools connection, where finder checks one\n"; !strings.Contains(release, want) {
			t.Errorf("%s: the tools connection's credential is not left in the comment:\n%s", row.scenario, release)
		}
		if _, ok := files[ManifestFileName]; ok != row.manifest {
			t.Errorf("%s: the draft writes a Manifest: %v, want %v", row.scenario, ok, row.manifest)
		}
		if row.manifest && !strings.HasSuffix(files[ManifestFileName],
			"        core/referenced-secret: helm/k8s-existing-secret\n") {
			t.Errorf("%s: the Manifest does not map core/referenced-secret:\n%s", row.scenario, files[ManifestFileName])
		}
	}
}

// TestThePlatformSetsItsValuesOnEveryTarget imports platformSource under
// releasingFormat setting one value of its own, for compose, for helm and for no
// target: on each the platform's entry sets it beneath its comment, after its
// image.
//
// 🔴 PLANTED DEFECT: in instanceEntry (platform.go), drop the line appending
// the platform's Values: every row fails, no value written.
func TestThePlatformSetsItsValuesOnEveryTarget(t *testing.T) {
	t.Parallel()
	format := releasingFormat("testplat/server:2.0.0")
	format.Platform.Values = []ReleaseValue{{Path: "infra.registry_enabled", Value: "true",
		Comment: "Lets the platform reach its registry."}}
	want := "        image: \"testplat/server:2.0.0\"\n" +
		"        # Lets the platform reach its registry.\n" +
		"        registry_enabled: true\n"
	for _, target := range []string{"compose", "helm", ""} {
		in := aggrega.ImportInput{Format: testFormatName, Catalog: platformCatalog(), Bytes: []byte(platformSource),
			Target: target}
		if release := releaseFile(Import(in, format, (&parseRecorder{}).parse)); !strings.Contains(release, want) {
			t.Errorf("target %q: the Release does not hold\n%s\nit reads\n%s", target, want, release)
		}
	}
}

// TestThePlatformPublishesItsPortsForCompose imports platformSource, and the
// same source twice under two names, under releasingFormat publishing 7000, for
// compose and for helm: for compose the platform's entry sets
// infra.published_port 7000 beneath the comment the kit writes for it, after its
// image, and a second instance of the platform sets 7001; for helm no entry sets
// one.
//
// 🔴 PLANTED DEFECT: in instanceEntry (platform.go), drop `Published:` from the
// entry's literal: the compose rows fail, no port written.
func TestThePlatformPublishesItsPortsForCompose(t *testing.T) {
	t.Parallel()
	format := releasingFormat("testplat/server:2.0.0")
	format.Platform.Published = []PublishedPort{{Port: 7000}}
	port := func(n int) string {
		return "        image: \"testplat/server:2.0.0\"\n" + publishedPortLines("        ", n)
	}
	two := []aggrega.ImportSource{{Name: "one.json", Bytes: []byte(platformSource)},
		{Name: "two.json", Bytes: []byte(platformSource)}}
	for _, row := range []struct {
		scenario, target string
		sources          []aggrega.ImportSource
		want             []string
	}{
		{"compose", "compose", nil, []string{port(7000)}},
		{"compose, two instances", "compose", two, []string{port(7000), port(7001)}},
		{"helm", "helm", nil, nil},
	} {
		in := aggrega.ImportInput{Format: testFormatName, Catalog: platformCatalog(), Bytes: []byte(platformSource),
			Sources: row.sources, Target: row.target}
		release := releaseFile(Import(in, format, (&parseRecorder{}).parse))
		for _, want := range row.want {
			if !strings.Contains(release, want) {
				t.Errorf("%s: the Release does not hold\n%s\nit reads\n%s", row.scenario, want, release)
			}
		}
		if got := strings.Count(release, "published_port: "); got != len(row.want) {
			t.Errorf("%s: the Release sets %d port(s), want %d:\n%s", row.scenario, got, len(row.want), release)
		}
	}
}
