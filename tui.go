package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	glamour "charm.land/glamour/v2"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

const barHeight = 2

var (
	barFilledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	barEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#292e42"))
	tuiStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	tuiDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

type tuiModel struct {
	viewport   viewport.Model
	width      int
	height     int
	thinking   string
	content    string
	displayPos int
	inThink    bool
	showThink  bool
	showInfo   bool
	styleName  string
	events     <-chan sseEvent
	done       bool
	dirty      bool
	renderer   *glamour.TermRenderer
	autoScroll bool
	meta       *streamMeta
}

type (
	sseMsg  sseEvent
	tickMsg time.Time
)

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		waitForSSE(m.events),
		tickCmd(),
	)
}

func waitForSSE(ch <-chan sseEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return sseMsg{Done: true}
		}
		return sseMsg(ev)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "G", "end":
			m.viewport.GotoBottom()
			m.autoScroll = true
			return m, nil
		case "g", "home":
			m.viewport.GotoTop()
			m.autoScroll = false
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - barHeight)
		// Recreate renderer for new wrap width
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.styleName),
			glamour.WithWordWrap(msg.Width),
		); err == nil {
			m.renderer = r
		}
		m.dirty = true

	case sseMsg:
		if msg.Meta != nil {
			m.meta = msg.Meta
		}
		if msg.Done {
			m.done = true
			m.dirty = true
			return m, nil
		}
		if msg.ReasoningContent != "" {
			m.thinking += msg.ReasoningContent
			m.dirty = true
		}
		if msg.Content != "" {
			td, cd, next := parseContent(msg.Content, m.inThink)
			m.thinking += td
			m.content += cd
			m.inThink = next
			m.dirty = true
		}
		return m, waitForSSE(m.events)

	case tickMsg:
		target := len(m.content)
		advanced := false
		if m.displayPos < target {
			if m.done {
				m.displayPos = target
			} else {
				m.displayPos = nextWordBoundary(m.content, m.displayPos)
			}
			advanced = true
		}
		if m.dirty || advanced {
			m.dirty = false
			md := buildMd(m.thinking, m.content[:m.displayPos], m.showThink)
			if md != "" {
				out, err := m.renderer.Render(md)
				if err != nil {
					out = md
				}
				m.viewport.SetContent(out)
				if m.autoScroll {
					m.viewport.GotoBottom()
				}
			}
		}
		return m, tickCmd()
	}

	// Delegate to viewport for scroll handling (j/k, pgup/pgdn, mouse, etc.)
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	// Stop auto-scroll if user scrolled away from bottom
	if m.viewport.ScrollPercent() < 1.0 {
		m.autoScroll = false
	}
	return m, cmd
}

func (m tuiModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Scroll progress bar
	pct := m.viewport.ScrollPercent()
	filled := min(int(math.Round(pct*float64(m.width))), m.width)
	empty := max(0, m.width-filled)
	b.WriteString(barFilledStyle.Render(strings.Repeat("━", filled)))
	b.WriteString(barEmptyStyle.Render(strings.Repeat("─", empty)))
	b.WriteString("\n")

	// Info bar
	status := tuiStatusStyle.Render("streaming...")
	if m.done {
		status = tuiStatusStyle.Render("done")
	}

	var info string
	if m.showInfo && m.meta != nil {
		info = tuiDimStyle.Render(formatMeta(m.meta))
	}

	keys := tuiDimStyle.Render("q/esc quit  j/k scroll  g/G top/bottom")

	if info != "" {
		left := status + "  " + info
		gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(keys))
		b.WriteString(left)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(keys)
	} else {
		gap := max(0, m.width-lipgloss.Width(status)-lipgloss.Width(keys))
		b.WriteString(status)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(keys)
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func formatMeta(m *streamMeta) string {
	var parts []string
	if m.Model != "" {
		parts = append(parts, m.Model)
	}
	if m.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%dt", m.TotalTokens))
	} else if m.EvalCount > 0 {
		parts = append(parts, fmt.Sprintf("%dt", m.PromptEvalCount+m.EvalCount))
	}
	if m.TotalDuration > 0 && m.EvalCount > 0 {
		tps := float64(m.EvalCount) / m.TotalDuration.Seconds()
		parts = append(parts, fmt.Sprintf("%.0f tok/s", tps))
	}
	return strings.Join(parts, "  ")
}

func runTUI(style string, width int, showThink, showInfo bool) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	ch := make(chan sseEvent, 256)
	go readEvents(ch)

	m := tuiModel{
		viewport:   viewport.New(),
		events:     ch,
		renderer:   r,
		showThink:  showThink,
		showInfo:   showInfo,
		styleName:  style,
		autoScroll: true,
	}

	// Read key input from /dev/tty since stdin is piped with SSE data
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("cannot open terminal for input: %w", err)
	}
	defer func() { _ = tty.Close() }()

	p := tea.NewProgram(m, tea.WithInput(tty))
	_, err = p.Run()
	return err
}
