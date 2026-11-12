// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package importer

import (
	"path"

	"github.com/aggrega-ai/aggrega-pdk-go/pkg/aggrega"
)

// fallbackSegment is the second segment of a drafted Aggregate's name where
// nothing the import is handed names the source.
const fallbackSegment = "flow"

// DraftName is the name an import's drafted Aggregate takes, and the drafted
// Release beside it: <namespace>/<segment>, where namespace is the one the
// target_namespace setting names, read as [TargetNamespace] reads it, and
// segment names the source. The segment is the first of these that is not empty
// once reduced to a name segment:
//
//   - the name the source gives itself. titles are the names the import's
//     sources give what each describes — a project's, a flow's, a workflow's, an
//     app's or an agent's name, in the field the format itself declares it in —
//     one per source or project the draft is made from, empty where one gives
//     none. They name the draft only where every one of them reduces to the same
//     segment, so a draft made from several sources that name themselves apart,
//     or from one that names itself and one that does not, is named by the next
//     rule;
//   - the name of what the import was handed, in.Name. Where the import was
//     handed a directory, it is the directory's name, whole: a directory hands
//     its files as named sources and no primary bytes, so a request carrying
//     named sources and empty primary bytes is read as a directory's, and
//     my.project names the draft my-project. Otherwise it is the name of the
//     file the import was handed on its own, its extension removed, so
//     compose.yaml names the draft compose;
//   - flow.
//
// So a directory whose sources name nothing is named after itself, as Compose
// names a project after the directory holding it.
//
// A name is reduced to a segment as [UniqueKey] reduces one to a key: lowercased,
// with every run of characters other than ASCII letters and digits written as
// one hyphen and the hyphens at either end dropped, so an accented letter is
// written as a hyphen too. Given one request and the same
// titles, it returns the same name.
//
// Every importer built on the package names its drafted Aggregate and Release
// through it, the package's own drafts included, so one source is named one way
// whichever importer drafts it.
func DraftName(in aggrega.ImportInput, titles ...string) string {
	return draftName(parseSettings(in.Settings), in, titles)
}

// draftName is [DraftName] over settings already parsed.
func draftName(settings importSettings, in aggrega.ImportInput, titles []string) string {
	return settings.TargetNamespace + "/" + draftSegment(in, titles)
}

// draftSegment is the second segment of the name [DraftName] returns.
func draftSegment(in aggrega.ImportInput, titles []string) string {
	if segment := agreedTitle(titles); segment != "" {
		return segment
	}
	if in.Name != "" {
		name := path.Base(in.Name)
		if !handedDirectory(in) {
			name = name[:len(name)-len(path.Ext(name))]
		}
		if segment := kebab(name); segment != "" {
			return segment
		}
	}
	return fallbackSegment
}

// handedDirectory reports whether a request is the import of a directory: its
// files arrive as named sources, and it carries no primary bytes, which carry a
// file imported on its own.
func handedDirectory(in aggrega.ImportInput) bool {
	return len(in.Bytes) == 0 && len(in.Sources) > 0
}

// agreedTitle is the segment every title reduces to, empty where there is no
// title, where one reduces to nothing, or where two reduce to different
// segments.
func agreedTitle(titles []string) string {
	agreed := ""
	for i, title := range titles {
		segment := kebab(title)
		if segment == "" || (i > 0 && segment != agreed) {
			return ""
		}
		agreed = segment
	}
	return agreed
}
