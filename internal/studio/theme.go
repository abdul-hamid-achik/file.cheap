package studio

import "charm.land/lipgloss/v2"

// Palette -- a tasteful dark theme with a cyan accent and magenta highlight.
var (
	colorInk         = lipgloss.Color("#E6E6E6")
	colorDim         = lipgloss.Color("#B7BCC6")
	colorMuted       = lipgloss.Color("#8A8F98")
	colorAccent      = lipgloss.Color("#66D9EF") // cyan
	colorAccent2     = lipgloss.Color("#F92672") // magenta
	colorGood        = lipgloss.Color("#A6E22E") // green
	colorWarn        = lipgloss.Color("#E6DB74") // yellow
	colorBad         = lipgloss.Color("#F92672")
	colorPanel       = lipgloss.Color("#3A3F4B")
	colorPanelActive = lipgloss.Color("#66D9EF")
	colorSelect      = lipgloss.Color("#2E5E6E")
	colorChipBg      = lipgloss.Color("#252932")
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	brandStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent2)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	dimStyle   = lipgloss.NewStyle().Foreground(colorDim)
	inkStyle   = lipgloss.NewStyle().Foreground(colorInk)
	errorStyle = lipgloss.NewStyle().Foreground(colorBad)
	warnStyle  = lipgloss.NewStyle().Foreground(colorWarn)
	goodStyle  = lipgloss.NewStyle().Foreground(colorGood)

	panelTitleStyle       = lipgloss.NewStyle().Bold(true).Foreground(colorDim)
	activePanelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	panelStyle        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorPanel).Padding(0, 1)
	focusedPanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorPanelActive).Padding(0, 1)

	// selectedRowStyle highlights the row under the cursor.
	selectedRowStyle = lipgloss.NewStyle().Foreground(colorInk).Background(colorSelect).Bold(true)

	// sectionStyle styles the group headers in the detail/provenance pane.
	sectionStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	// colHeaderStyle styles the stash-list column header row.
	colHeaderStyle = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	// colHeaderActiveStyle marks the column the list is currently sorted by.
	colHeaderActiveStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true)

	zstChipStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1E1E1E")).Background(colorAccent).Padding(0, 1).Bold(true)
	tagChipStyle  = lipgloss.NewStyle().Foreground(colorAccent).Background(colorChipBg).Padding(0, 1)
	warnChipStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1E1E1E")).Background(colorWarn).Padding(0, 1).Bold(true)
	expChipStyle  = lipgloss.NewStyle().Foreground(colorWarn).Background(colorChipBg).Padding(0, 1)

	keyStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// timeline view
	timeStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	frameStyle = lipgloss.NewStyle().Foreground(colorAccent2)
	labelStyle = lipgloss.NewStyle().Foreground(colorMuted)
)

// plural picks the singular or plural form based on n.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func clamp(n, lo, hi int) int {
	if hi < lo {
		lo = hi // tolerate inverted bounds (e.g. very small terminal widths)
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// keyHint formats a "key action" pair for the footer hint line.
func keyHint(key, action string) string {
	return keyStyle.Render(key) + " " + hintStyle.Render(action)
}

// scoreStyle colors a search score by its strength relative to the best hit, so
// the strongest matches read green, mid cyan, and weak ones dim.
func scoreStyle(score, top float64) lipgloss.Style {
	if top <= 0 {
		return mutedStyle
	}
	switch r := score / top; {
	case r >= 0.66:
		return goodStyle
	case r >= 0.33:
		return titleStyle
	default:
		return mutedStyle
	}
}

// bundleChipStyle returns a colored chip style for a bundle type.
func bundleChipStyle(bundleType string) lipgloss.Style {
	if bundleType == "vidtrace" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#1E1E1E")).Background(colorAccent2).Padding(0, 1).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(colorMuted).Background(colorChipBg).Padding(0, 1)
}

// toolStyle colors the TOOL column by producing tool, so rows are scannable.
func toolStyle(tool string) lipgloss.Style {
	switch tool {
	case "vidtrace":
		return lipgloss.NewStyle().Foreground(colorAccent2) // magenta
	case "vecgrep":
		return lipgloss.NewStyle().Foreground(colorGood) // green
	case "tinyvault", "tvault":
		return lipgloss.NewStyle().Foreground(colorWarn) // yellow
	case "", "generic":
		return lipgloss.NewStyle().Foreground(colorMuted)
	default:
		return lipgloss.NewStyle().Foreground(colorAccent) // cyan
	}
}
