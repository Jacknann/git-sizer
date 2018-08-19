package sizes

import (
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"

	"github.com/spf13/pflag"
)

func (s BlobSize) String() string {
	return fmt.Sprintf("blob_size=%d", s.Size)
}

func (s TreeSize) String() string {
	return fmt.Sprintf(
		"max_path_depth=%d, max_path_length=%d, "+
			"expanded_tree_count=%d, "+
			"expanded_blob_count=%d, expanded_blob_size=%d, "+
			"expanded_link_count=%d, expanded_submodule_count=%d",
		s.MaxPathDepth, s.MaxPathLength,
		s.ExpandedTreeCount,
		s.ExpandedBlobCount, s.ExpandedBlobSize,
		s.ExpandedLinkCount, s.ExpandedSubmoduleCount,
	)
}

func (s CommitSize) String() string {
	return fmt.Sprintf(
		"max_ancestor_depth=%d",
		s.MaxAncestorDepth,
	)
}

func (s TagSize) String() string {
	return fmt.Sprintf("tag_depth=%d", s.TagDepth)
}

func (s HistorySize) String() string {
	return fmt.Sprintf(
		"unique_commit_count=%d, unique_commit_count = %d, max_commit_size = %d, "+
			"max_history_depth=%d, max_parent_count=%d, "+
			"unique_tree_count=%d, unique_tree_entries=%d, max_tree_entries=%d, "+
			"unique_blob_count=%d, unique_blob_size=%d, max_blob_size=%d, "+
			"unique_tag_count=%d, "+
			"reference_count=%d, "+
			"max_path_depth=%d, max_path_length=%d, "+
			"max_expanded_tree_count=%d, "+
			"max_expanded_blob_count=%d, max_expanded_blob_size=%d, "+
			"max_expanded_link_count=%d, max_expanded_submodule_count=%d",
		s.UniqueCommitCount, s.UniqueCommitSize, s.MaxCommitSize,
		s.MaxHistoryDepth, s.MaxParentCount,
		s.UniqueTreeCount, s.UniqueTreeEntries, s.MaxTreeEntries,
		s.UniqueBlobCount, s.UniqueBlobSize, s.MaxBlobSize,
		s.UniqueTagCount,
		s.ReferenceCount,
		s.MaxPathDepth, s.MaxPathLength,
		s.MaxExpandedTreeCount, s.MaxExpandedBlobCount,
		s.MaxExpandedBlobSize, s.MaxExpandedLinkCount,
		s.MaxExpandedSubmoduleCount,
	)
}

const (
	spaces = "                            "
	stars  = "******************************"
)

// Zero or more lines in the tabular output.
type tableContents interface {
	Emit(t *table, buf io.Writer, indent int)
}

// A section of lines in the tabular output, consisting of a header
// and a number of bullet lines. The lines in a section can themselves
// be bulletized, in which case the header becomes a top-level bullet
// and the lines become second-level bullets.
type section struct {
	name     string
	contents []tableContents
}

func newSection(name string, contents ...tableContents) *section {
	return &section{
		name:     name,
		contents: contents,
	}
}

func (s *section) Emit(t *table, buf io.Writer, indent int) {
	var linesBuf bytes.Buffer
	for _, c := range s.contents {
		var cBuf bytes.Buffer
		c.Emit(t, &cBuf, indent+1)

		if indent == -1 && linesBuf.Len() > 0 && cBuf.Len() > 0 {
			// The top-level section emits blank lines between its
			// subsections:
			t.emitBlankRow(&linesBuf)
		}

		fmt.Fprint(&linesBuf, cBuf.String())
	}

	if linesBuf.Len() == 0 {
		if indent == -1 {
			fmt.Fprintln(buf, "No problems above the current threshold were found")
		}
		return
	}

	// There's output, so emit the section header first:
	if indent == -1 {
		// As a special case, the top-level section doesn't have its
		// own header, but prints the table header:
		fmt.Fprint(buf, t.generateHeader())
	} else {
		t.formatSectionHeader(buf, indent, s.name)
	}

	fmt.Fprint(buf, linesBuf.String())
}

// A line containing data in the tabular output.
type item struct {
	name     string
	path     *Path
	value    counts.Humaner
	prefixes []counts.Prefix
	unit     string
	scale    float64
}

func newItem(
	name string,
	path *Path,
	value counts.Humaner,
	prefixes []counts.Prefix,
	unit string,
	scale float64,
) *item {
	return &item{
		name:     name,
		path:     path,
		value:    value,
		prefixes: prefixes,
		unit:     unit,
		scale:    scale,
	}
}

func (l *item) Emit(t *table, buf io.Writer, indent int) {
	levelOfConcern, interesting := l.levelOfConcern(t.threshold)
	if !interesting {
		return
	}
	valueString, unitString := l.value.Human(l.prefixes, l.unit)
	t.formatRow(
		buf, indent,
		l.name, t.footnotes.CreateCitation(l.Footnote(t.nameStyle)),
		valueString, unitString,
		levelOfConcern,
	)
}

func (l *item) Footnote(nameStyle NameStyle) string {
	if l.path == nil || l.path.OID == git.NullOID {
		return ""
	}
	switch nameStyle {
	case NameStyleNone:
		return ""
	case NameStyleHash:
		return l.path.OID.String()
	case NameStyleFull:
		return l.path.String()
	default:
		panic("unexpected NameStyle")
	}
}

// If this item's alert level is at least as high as the threshold,
// return the string that should be used as its "level of concern" and
// `true`; otherwise, return `"", false`.
func (l *item) levelOfConcern(threshold Threshold) (string, bool) {
	alert := Threshold(float64(l.value.ToUint64()) / l.scale)
	if alert < threshold {
		return "", false
	}
	if alert > 30 {
		return "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", true
	}
	return stars[:int(alert)], true
}

type Threshold float64

// Methods to implement pflag.Value:
func (t *Threshold) String() string {
	if t == nil {
		return "UNSET"
	} else {
		switch *t {
		case 0:
			return "--verbose"
		case 1:
			return "--threshold=1"
		case 30:
			return "--critical"
		default:
			return fmt.Sprintf("--threshold=%g", *t)
		}
	}
}

func (t *Threshold) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("error parsing floating-point value %q: %s", s, err)
	}
	*t = Threshold(v)
	return nil
}

func (t *Threshold) Type() string {
	return "threshold"
}

// A `pflag.Value` that can be used as a boolean option that sets a
// `Threshold` variable to a fixed value. For example,
//
//		pflag.Var(
//			sizes.NewThresholdFlagValue(&threshold, 30),
//			"critical", "only report critical statistics",
//		)
//
// adds a `--critical` flag that sets `threshold` to 30.
type thresholdFlagValue struct {
	b         bool
	threshold *Threshold
	value     Threshold
}

func NewThresholdFlagValue(threshold *Threshold, value Threshold) pflag.Value {
	return &thresholdFlagValue{false, threshold, value}
}

func (v *thresholdFlagValue) String() string {
	return strconv.FormatBool(v.b)
}

func (v *thresholdFlagValue) Set(s string) error {
	value, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	v.b = value
	if value {
		*v.threshold = v.value
	} else {
		*v.threshold = 1
	}
	return nil
}

func (v *thresholdFlagValue) Type() string {
	return "bool"
}

type NameStyle int

const (
	NameStyleNone NameStyle = iota
	NameStyleHash
	NameStyleFull
)

// Methods to implement pflag.Value:
func (n *NameStyle) String() string {
	if n == nil {
		return "UNSET"
	} else {
		switch *n {
		case NameStyleNone:
			return "none"
		case NameStyleHash:
			return "hash"
		case NameStyleFull:
			return "full"
		default:
			panic("Unexpected NameStyle value")
		}
	}
}

func (n *NameStyle) Set(s string) error {
	switch s {
	case "none":
		*n = NameStyleNone
	case "hash", "sha-1", "sha1":
		*n = NameStyleHash
	case "full":
		*n = NameStyleFull
	default:
		return fmt.Errorf("not a valid name style: %v", s)
	}
	return nil
}

func (n *NameStyle) Type() string {
	return "nameStyle"
}

type table struct {
	contents  tableContents
	threshold Threshold
	nameStyle NameStyle
	footnotes *Footnotes
}

func (s HistorySize) TableString(threshold Threshold, nameStyle NameStyle) string {
	t := table{
		contents:  s.Contents(),
		threshold: threshold,
		nameStyle: nameStyle,
		footnotes: NewFootnotes(),
	}

	buf := &bytes.Buffer{}
	t.contents.Emit(&t, buf, -1)
	linesString := buf.String()
	return linesString + t.footnotes.String()
}

func (t *table) generateHeader() string {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "| Name                         | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ---------------------------- | --------- | ------------------------------ |")
	return buf.String()
}

func (t *table) emitBlankRow(buf io.Writer) {
	t.formatRow(buf, 0, "", "", "", "", "")
}

func (t *table) formatSectionHeader(buf io.Writer, indent int, name string) {
	t.formatRow(buf, indent, name, "", "", "", "")
}

func (t *table) formatRow(
	buf io.Writer, indent int,
	name, citation, valueString, unitString, levelOfConcern string,
) {
	prefix := ""
	if indent != 0 {
		prefix = spaces[:2*(indent-1)] + "* "
	}
	spacer := ""
	l := len(prefix) + len(name) + len(citation)
	if l < 28 {
		spacer = spaces[:28-l]
	}
	fmt.Fprintf(
		buf, "| %s%s%s%s | %5s %-3s | %-30s |\n",
		prefix, name, spacer, citation, valueString, unitString, levelOfConcern,
	)
}

func (s HistorySize) Contents() tableContents {
	S := newSection
	I := newItem
	metric := counts.MetricPrefixes
	binary := counts.BinaryPrefixes
	return S(
		"",
		S(
			"Overall repository size",
			S(
				"Commits",
				I("Count", nil, s.UniqueCommitCount, metric, " ", 500e3),
				I("Total size", nil, s.UniqueCommitSize, binary, "B", 250e6),
			),

			S(
				"Trees",
				I("Count", nil, s.UniqueTreeCount, metric, " ", 1.5e6),
				I("Total size", nil, s.UniqueTreeSize, binary, "B", 2e9),
				I("Total tree entries", nil, s.UniqueTreeEntries, metric, " ", 50e6),
			),

			S(
				"Blobs",
				I("Count", nil, s.UniqueBlobCount, metric, " ", 1.5e6),
				I("Total size", nil, s.UniqueBlobSize, binary, "B", 10e9),
			),

			S(
				"Annotated tags",
				I("Count", nil, s.UniqueTagCount, metric, " ", 25e3),
			),

			S(
				"References",
				I("Count", nil, s.ReferenceCount, metric, " ", 25e3),
			),
		),

		S("Biggest objects",
			S("Commits",
				I("Maximum size", s.MaxCommitSizeCommit, s.MaxCommitSize, binary, "B", 50e3),
				I("Maximum parents", s.MaxParentCountCommit, s.MaxParentCount, metric, " ", 10),
			),

			S("Trees",
				I("Maximum entries", s.MaxTreeEntriesTree, s.MaxTreeEntries, metric, " ", 1000),
			),

			S("Blobs",
				I("Maximum size", s.MaxBlobSizeBlob, s.MaxBlobSize, binary, "B", 10e6),
			),
		),

		S("History structure",
			I("Maximum history depth", nil, s.MaxHistoryDepth, metric, " ", 500e3),
			I("Maximum tag depth", s.MaxTagDepthTag, s.MaxTagDepth, metric, " ", 1.001),
		),

		S("Biggest checkouts",
			I("Number of directories", s.MaxExpandedTreeCountTree, s.MaxExpandedTreeCount, metric, " ", 2000),
			I("Maximum path depth", s.MaxPathDepthTree, s.MaxPathDepth, metric, " ", 10),
			I("Maximum path length", s.MaxPathLengthTree, s.MaxPathLength, binary, "B", 100),

			I("Number of files", s.MaxExpandedBlobCountTree, s.MaxExpandedBlobCount, metric, " ", 50e3),
			I("Total size of files", s.MaxExpandedBlobSizeTree, s.MaxExpandedBlobSize, binary, "B", 1e9),

			I("Number of symlinks", s.MaxExpandedLinkCountTree, s.MaxExpandedLinkCount, metric, " ", 25e3),

			I("Number of submodules", s.MaxExpandedSubmoduleCountTree, s.MaxExpandedSubmoduleCount, metric, " ", 100),
		),
	)
}
