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
		tickCmd(0),
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

func tickCmd(backlog int) tea.Cmd {
	// Scale tick speed with backlog: more behind = faster reveal
	// 30ms at idle, down to 5ms when far behind
	delay := 30 * time.Millisecond
	if backlog > 0 {
		delay = max(5*time.Millisecond, delay/time.Duration(1+backlog/50))
	}
	return tea.Tick(delay, func(t time.Time) tea.Msg {
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
		return m, tickCmd(target - m.displayPos)
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

// --- Inline mode (no alt screen) ---

type inlineModel struct {
	thinking   string
	content    string
	displayPos int
	inThink    bool
	showThink  bool
	quitting   bool
	styleName  string
	width      int
	height     int
	events     <-chan sseEvent
	done       bool
	renderer   *glamour.TermRenderer
	meta       *streamMeta
	rendered   string
}

func (m inlineModel) Init() tea.Cmd {
	return tea.Batch(
		waitForSSE(m.events),
		tickCmd(0),
	)
}

func (m inlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.styleName),
			glamour.WithWordWrap(msg.Width),
		); err == nil {
			m.renderer = r
		}

	case sseMsg:
		if msg.Meta != nil {
			m.meta = msg.Meta
		}
		if msg.Done {
			m.done = true
			return m, nil
		}
		if msg.ReasoningContent != "" {
			m.thinking += msg.ReasoningContent
		}
		if msg.Content != "" {
			td, cd, next := parseContent(msg.Content, m.inThink)
			m.thinking += td
			m.content += cd
			m.inThink = next
		}
		return m, waitForSSE(m.events)

	case tickMsg:
		target := len(m.content)
		if m.displayPos < target {
			if m.done {
				m.displayPos = target
			} else {
				m.displayPos = nextWordBoundary(m.content, m.displayPos)
			}
			md := buildMd(m.thinking, m.content[:m.displayPos], m.showThink)
			if md != "" {
				out, err := m.renderer.Render(md)
				if err != nil {
					out = md
				}
				m.rendered = out
			}
		}
		if m.done && m.displayPos >= target {
			m.quitting = true
			return m, tea.Quit
		}
		return m, tickCmd(target - m.displayPos)
	}

	return m, nil
}

func (m inlineModel) View() tea.View {
	if m.quitting {
		// Clear bubbletea's view area so the post-exit print is clean
		return tea.NewView("")
	}

	out := m.rendered
	// Cap to terminal height so bubbletea never pushes into scrollback
	if m.height > 0 {
		lines := strings.Split(out, "\n")
		if len(lines) > m.height {
			lines = lines[len(lines)-m.height:]
		}
		out = strings.Join(lines, "\n")
	}

	return tea.NewView(out)
}

func openTTY() (*os.File, func(), error) {
	tty, err := os.Open("/dev/tty")
	if err == nil {
		return tty, func() { tty.Close() }, nil
	}
	// Fallback: pipe gives no input but supports epoll;
	// Ctrl+C is handled via SIGINT.
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open terminal for input: %w", err)
	}
	return r, func() { r.Close(); w.Close() }, nil
}

func runInline(style string, width int, showThink, showInfo bool) error {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return err
	}

	ch := make(chan sseEvent, 256)
	go readEvents(ch)

	m := inlineModel{
		events:    ch,
		renderer:  r,
		showThink: showThink,
		styleName: style,
		width:     width,
	}

	tty, cleanup, err := openTTY()
	if err != nil {
		return err
	}
	defer cleanup()

	p := tea.NewProgram(m, tea.WithInput(tty))
	result, err := p.Run()
	if err != nil {
		return err
	}

	// Print the full rendered output so it stays in scrollback
	if im, ok := result.(inlineModel); ok {
		md := buildMd(im.thinking, im.content, im.showThink)
		if md != "" {
			out, renderErr := im.renderer.Render(md)
			if renderErr != nil {
				out = md
			}
			fmt.Print(out)
		}
		if showInfo && im.meta != nil {
			printMeta(im.meta)
		}
	}

	return nil
}

// --- Alt-screen mode ---

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
	tty, cleanup, err := openTTY()
	if err != nil {
		return err
	}
	defer cleanup()

	p := tea.NewProgram(m, tea.WithInput(tty))
	_, err = p.Run()
	return err
}
