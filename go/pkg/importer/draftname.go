// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

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
//   - the stem of the file the import was handed on its own: in.Name with its
//     extension removed. A directory hands no such name;
//   - flow.
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
		base := path.Base(in.Name)
		if segment := kebab(base[:len(base)-len(path.Ext(base))]); segment != "" {
			return segment
		}
	}
	return fallbackSegment
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
