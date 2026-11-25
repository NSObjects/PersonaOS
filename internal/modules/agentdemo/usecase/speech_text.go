package usecase

import (
	"regexp"
	"strings"
)

// speechTextFilter is deliberately a small streaming normalizer, not a full
// Markdown renderer. A turn owns its state; raw Agent text is never changed.
type speechTextFilter struct {
	lineStart  bool
	prefix     string
	inLabel    bool
	afterLabel bool
	linkDepth  int
}

var (
	spokenPrefix    = regexp.MustCompile(`^[ \t]*(#{1,6}|[-+*]|>|[0-9]{1,9}[.)])[ \t]+$`)
	prefixCandidate = regexp.MustCompile(`^[ \t]*(#{0,6}|[-+*]|>|[0-9]{1,9}[.)]?)?$`)
)

func newSpeechTextFilter() *speechTextFilter {
	return &speechTextFilter{lineStart: true}
}

func (f *speechTextFilter) Write(delta string) string {
	var out strings.Builder
	for _, r := range delta {
		if r == '\n' {
			f.flushPrefix(&out)
			f.lineStart = true
			f.inLabel, f.afterLabel, f.linkDepth = false, false, 0
			out.WriteRune(r)
			continue
		}
		if f.lineStart {
			f.prefix += string(r)
			if spokenPrefix.MatchString(f.prefix) {
				f.prefix = ""
				f.lineStart = false
				continue
			}
			// Only delay a small potential heading/list marker, not a line
			// or sentence. Ordinary spoken text streams immediately.
			if len(f.prefix) <= 32 && prefixCandidate.MatchString(f.prefix) {
				continue
			}
			f.flushPrefix(&out)
			f.lineStart = false
			continue
		}
		f.writeInline(&out, r)
	}
	return out.String()
}

func (f *speechTextFilter) flushPrefix(out *strings.Builder) {
	for _, r := range f.prefix {
		f.writeInline(out, r)
	}
	f.prefix = ""
}

func (f *speechTextFilter) writeInline(out *strings.Builder, r rune) {
	if f.linkDepth > 0 {
		switch r {
		case '(':
			f.linkDepth++
		case ')':
			f.linkDepth--
		}
		return
	}
	if f.afterLabel {
		f.afterLabel = false
		if r == '(' {
			f.linkDepth = 1
			return
		}
	}
	switch r {
	case '*', '_', '~', '`':
		// Emphasis, strikethrough and inline code delimiters.
	case '[':
		f.inLabel = true
	case ']':
		if f.inLabel {
			f.inLabel, f.afterLabel = false, true
		} else {
			out.WriteRune(r)
		}
	default:
		out.WriteRune(r)
	}
}

func (f *speechTextFilter) Finish() string {
	var out strings.Builder
	f.flushPrefix(&out)
	return out.String()
}

func spokenText(text string) string {
	filter := newSpeechTextFilter()
	return strings.TrimSpace(filter.Write(text) + filter.Finish())
}
