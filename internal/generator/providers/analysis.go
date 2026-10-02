package providers

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// partRecorder captures, while a file is being rendered, the byte range each
// section occupied in the output builder. The ranges are sliced out of the
// finished string once by flush, so recording a 200-skill tree does not
// materialize the builder 200 times.
//
// A nil *partRecorder is a working no-op: the renderer records unconditionally
// and pays nothing when the `tokens` command has not asked for an analysis.
type partRecorder struct {
	analysis *config.OutputAnalysis
	ranges   []partRange
	literals []config.OutputPart
}

type partRange struct {
	kind       config.PartKind
	label      string
	sourcePath string
	start      int
	end        int
}

// newPartRecorder returns a recorder writing into analysis, or nil when
// analysis is nil.
func newPartRecorder(analysis *config.OutputAnalysis) *partRecorder {
	if analysis == nil {
		return nil
	}
	return &partRecorder{analysis: analysis}
}

// mark returns the builder's current length, the start offset of the section
// about to be written.
func (r *partRecorder) mark(builder *strings.Builder) int {
	if r == nil {
		return 0
	}
	return builder.Len()
}

// Mark implements rulefiles.Recorder.
func (r *partRecorder) Mark(builder *strings.Builder) int { return r.mark(builder) }

// Section implements rulefiles.Recorder.
func (r *partRecorder) Section(kind config.PartKind, label, sourcePath string, start int, builder *strings.Builder) {
	r.section(kind, label, sourcePath, start, builder)
}

// section records the byte range [start, builder.Len()) as one part.
func (r *partRecorder) section(kind config.PartKind, label, sourcePath string, start int, builder *strings.Builder) {
	if r == nil {
		return
	}
	end := builder.Len()
	if end <= start {
		return
	}
	r.ranges = append(r.ranges, partRange{
		kind:       kind,
		label:      label,
		sourcePath: sourcePath,
		start:      start,
		end:        end,
	})
}

// literal records a part whose content is not a contiguous slice of the output —
// a frontmatter field value, for instance, which yaml.Marshal may have quoted,
// folded or reordered.
func (r *partRecorder) literal(kind config.PartKind, label, sourcePath, content string) {
	if r == nil || content == "" {
		return
	}
	r.literals = append(r.literals, config.OutputPart{
		Kind:       kind,
		Label:      label,
		SourcePath: sourcePath,
		Content:    content,
	})
}

// flush resolves every recorded range against the finished output and appends
// the parts to the analysis, ranges first (in render order) then literals.
func (r *partRecorder) flush(rendered string) {
	if r == nil {
		return
	}
	for _, span := range r.ranges {
		if span.start < 0 || span.end > len(rendered) {
			continue
		}
		r.analysis.AddPart(span.kind, span.label, span.sourcePath, rendered[span.start:span.end])
	}
	for _, part := range r.literals {
		r.analysis.AddPart(part.Kind, part.Label, part.SourcePath, part.Content)
	}
	r.ranges = nil
	r.literals = nil
}

// outputKindForType maps a provider DSL output type onto the analysis kind.
func outputKindForType(typ string) config.OutputKind {
	switch typ {
	case OutputTypeRules:
		return config.OutputKindRuleFile
	case OutputTypeSkills:
		return config.OutputKindSkill
	case OutputTypeAgents:
		return config.OutputKindAgent
	case OutputTypeCommands:
		return config.OutputKindCommand
	default:
		return config.OutputKindUnknown
	}
}

// partKindForRootSection maps a root.sections entry onto the analysis part kind.
// Rules and context are recorded per entry rather than per section, so they are
// absent here.
func partKindForRootSection(section string) config.PartKind {
	switch section {
	case SectionRootHeader:
		return config.PartKindRootHeader
	case SectionRootTitle:
		return config.PartKindRootTitle
	case SectionRootDescription:
		return config.PartKindRootDescription
	case SectionRootAgentsDelegation:
		return config.PartKindRootAgentsRoster
	default:
		return config.PartKind(section)
	}
}

// frontmatterString reads a frontmatter map value as a string, returning "" for
// a missing key or a non-string value.
func frontmatterString(frontmatter map[string]any, key string) string {
	value, ok := frontmatter[key].(string)
	if !ok {
		return ""
	}
	return value
}
