package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type calendarState struct {
	year  int
	month time.Month
	day   int
}

type model struct {
	styles styleSet
	state  calendarState
	width  int
}

const (
	monthInnerWidth = 20
	monthGapWidth   = 4
	maxColumns      = 4
	defaultColumns  = 3
)

func initialModel() model {
	now := time.Now()
	return model{
		styles: newStyles(),
		state: calendarState{
			year:  now.Year(),
			month: now.Month(),
			day:   now.Day(),
		},
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "left", "h":
			adjustDay(&m.state, -1)
		case "right", "l":
			adjustDay(&m.state, 1)
		case "up", "k":
			adjustDay(&m.state, -7)
		case "down", "j":
			adjustDay(&m.state, 7)
		case "n":
			adjustMonth(&m.state, 1)
		case "p":
			adjustMonth(&m.state, -1)
		case "N":
			adjustYear(&m.state, 1)
		case "P":
			adjustYear(&m.state, -1)
		case "t", "T":
			now := time.Now()
			m.state.year, m.state.month, m.state.day = now.Year(), now.Month(), now.Day()
		}
	}
	clampDay(&m.state)
	return m, nil
}

func (m model) View() string {
	months := make([][]string, 0, 12)
	for month := time.January; month <= time.December; month++ {
		months = append(months, renderMonthLines(m.state.year, month, m.state, m.styles))
	}

	cols := m.columns()
	gap := strings.Repeat(" ", monthGapWidth)

	var b strings.Builder
	for start := 0; start < len(months); start += cols {
		end := start + cols
		if end > len(months) {
			end = len(months)
		}
		rowLines := joinMonthRow(months[start:end], gap)
		for i, line := range rowLines {
			b.WriteString(line)
			if i < len(rowLines)-1 {
				b.WriteString("\n")
			}
		}
		if end < len(months) {
			b.WriteString("\n\n")
		}
	}

	if b.Len() > 0 {
		b.WriteString("\n\n")
	}

	selected := fmt.Sprintf("Selected: %04d-%02d-%02d", m.state.year, int(m.state.month), m.state.day)
	b.WriteString(m.styles.footer.Render(selected))
	b.WriteString("\n")
	help := "Arrows/Vim: Move  n/p: Next/Prev month  N/P: Next/Prev year  t: Today  q: Quit"
	b.WriteString(m.styles.help.Render(help))

	return b.String()
}

func (m model) columns() int {
	if m.width <= 0 {
		return defaultColumns
	}
	for cols := maxColumns; cols >= 1; cols-- {
		required := cols*monthInnerWidth + (cols-1)*monthGapWidth
		if m.width >= required {
			return cols
		}
	}
	return 1
}

func joinMonthRow(months [][]string, gap string) []string {
	if len(months) == 0 {
		return nil
	}
	height := len(months[0])
	lines := make([]string, height)
	for _, month := range months {
		for i := 0; i < height; i++ {
			if lines[i] != "" {
				lines[i] += gap
			}
			lines[i] += month[i]
		}
	}
	return lines
}

func renderMonthLines(year int, month time.Month, state calendarState, styles styleSet) []string {
	lines := make([]string, 0, 8)
	title := fmt.Sprintf("%s %d", month.String(), year)
	lines = append(lines, styles.header.Render(centerText(title, monthInnerWidth)))
	lines = append(lines, styles.weekday.Render("Su Mo Tu We Th Fr Sa"))

	firstWeekday := int(time.Date(year, month, 1, 0, 0, 0, 0, time.Local).Weekday())
	daysInMonth := daysIn(year, month)

	for week := 0; week < 6; week++ {
		var weekBuilder strings.Builder
		for weekday := 0; weekday < 7; weekday++ {
			index := week*7 + weekday
			currentDay := index - firstWeekday + 1
			var text string
			style := styles.day
			if index < firstWeekday || currentDay > daysInMonth {
				text = "  "
			} else {
				text = fmt.Sprintf("%2d", currentDay)
				isWeekend := weekday == 0 || weekday == 6
				if isWeekend {
					style = styles.weekend
				}
				if currentDay == state.day && month == state.month && year == state.year {
					if isWeekend {
						style = styles.selectedWeekend
					} else {
						style = styles.selectedDay
					}
				}
			}
			weekBuilder.WriteString(style.Render(text))
			if weekday < 6 {
				weekBuilder.WriteString(" ")
			}
		}
		lines = append(lines, weekBuilder.String())
	}

	return lines
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := width - len(text)
	left := padding / 2
	right := padding - left
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		panic(err)
	}
}

func adjustDay(state *calendarState, delta int) {
	t := time.Date(state.year, state.month, state.day, 0, 0, 0, 0, time.Local)
	t = t.AddDate(0, 0, delta)
	state.year, state.month, state.day = t.Year(), t.Month(), t.Day()
}

func adjustMonth(state *calendarState, delta int) {
	t := time.Date(state.year, state.month, state.day, 0, 0, 0, 0, time.Local)
	t = t.AddDate(0, delta, 0)
	state.year, state.month, state.day = t.Year(), t.Month(), t.Day()
}

func adjustYear(state *calendarState, delta int) {
	t := time.Date(state.year, state.month, state.day, 0, 0, 0, 0, time.Local)
	t = t.AddDate(delta, 0, 0)
	state.year, state.month, state.day = t.Year(), t.Month(), t.Day()
}

func clampDay(state *calendarState) {
	days := daysIn(state.year, state.month)
	if state.day > days {
		state.day = days
	}
	if state.day < 1 {
		state.day = 1
	}
}

func daysIn(year int, month time.Month) int {
	t := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local)
	return t.Day()
}

type styleSet struct {
	header          lipgloss.Style
	weekday         lipgloss.Style
	day             lipgloss.Style
	weekend         lipgloss.Style
	selectedDay     lipgloss.Style
	selectedWeekend lipgloss.Style
	footer          lipgloss.Style
	help            lipgloss.Style
}

func newStyles() styleSet {
	base := lipgloss.NewStyle().Padding(0).Margin(0)

	return styleSet{
		header:          base.Copy().Foreground(lipgloss.Color("213")).Bold(true),
		weekday:         base.Copy().Foreground(lipgloss.Color("111")).Bold(true),
		day:             base.Copy().Foreground(lipgloss.Color("252")),
		weekend:         base.Copy().Foreground(lipgloss.Color("210")),
		selectedDay:     base.Copy().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true),
		selectedWeekend: base.Copy().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("198")).Bold(true),
		footer:          base.Copy().Foreground(lipgloss.Color("248")),
		help:            base.Copy().Foreground(lipgloss.Color("244")),
	}
}
