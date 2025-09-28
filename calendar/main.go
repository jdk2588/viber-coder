package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	picker pickerState
}

const (
	monthInnerWidth = 20
	monthGapWidth   = 4
	maxColumns      = 4
	defaultColumns  = 3
)

type pickerType int

const (
	pickerNone pickerType = iota
	pickerYear
	pickerMonth
)

type pickerState struct {
	active      pickerType
	yearCursor  int
	monthCursor int
	yearBuffer  string
	monthBuffer string
}

func (p *pickerState) openYear(year int) {
	p.active = pickerYear
	p.yearCursor = year
	p.yearBuffer = ""
	p.monthBuffer = ""
}

func (p *pickerState) openMonth(month time.Month) {
	p.active = pickerMonth
	p.monthCursor = int(month) - 1
	p.monthBuffer = ""
	p.yearBuffer = ""
}

func (p *pickerState) close() {
	p.active = pickerNone
	p.yearBuffer = ""
	p.monthBuffer = ""
}

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
		if handlePickerInput(&m, msg) {
			break
		}
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
		case "y", "Y":
			if m.picker.active == pickerYear {
				m.picker.close()
			} else {
				m.picker.openYear(m.state.year)
			}
		case "m", "M":
			if m.picker.active == pickerMonth {
				m.picker.close()
			} else {
				m.picker.openMonth(m.state.month)
			}
		}
	}
	clampDay(&m.state)
	return m, nil
}

func handlePickerInput(m *model, msg tea.KeyMsg) bool {
	if m.picker.active == pickerNone {
		return false
	}

	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			switch m.picker.active {
			case pickerYear:
				handleYearRunes(&m.picker, msg.Runes)
			case pickerMonth:
				handleMonthRunes(&m.picker, msg.Runes)
			}
		}
		return true
	}

	switch msg.String() {
	case "backspace", "ctrl+h":
		switch m.picker.active {
		case pickerYear:
			if m.picker.yearBuffer != "" {
				m.picker.yearBuffer = trimLastRune(m.picker.yearBuffer)
				if val, err := strconv.Atoi(m.picker.yearBuffer); err == nil {
					m.picker.yearCursor = val
				}
			}
		case pickerMonth:
			if m.picker.monthBuffer != "" {
				m.picker.monthBuffer = trimLastRune(m.picker.monthBuffer)
				if val, err := strconv.Atoi(m.picker.monthBuffer); err == nil {
					if val >= 1 && val <= 12 {
						m.picker.monthCursor = val - 1
					}
				}
			}
		}
		return true
	case "esc":
		m.picker.close()
		return true
	case "enter":
		switch m.picker.active {
		case pickerYear:
			m.state.year = m.picker.yearCursor
		case pickerMonth:
			m.state.month = time.Month(m.picker.monthCursor + 1)
		}
		m.picker.close()
		return true
	case "up", "k":
		if m.picker.active == pickerYear {
			m.picker.yearCursor--
			m.picker.yearBuffer = ""
		} else if m.picker.active == pickerMonth {
			m.picker.monthCursor = (m.picker.monthCursor + 11) % 12
			m.picker.monthBuffer = ""
		}
		return true
	case "down", "j":
		if m.picker.active == pickerYear {
			m.picker.yearCursor++
			m.picker.yearBuffer = ""
		} else if m.picker.active == pickerMonth {
			m.picker.monthCursor = (m.picker.monthCursor + 1) % 12
			m.picker.monthBuffer = ""
		}
		return true
	case "pgup":
		if m.picker.active == pickerYear {
			m.picker.yearCursor -= 10
			m.picker.yearBuffer = ""
			return true
		}
	case "pgdown":
		if m.picker.active == pickerYear {
			m.picker.yearCursor += 10
			m.picker.yearBuffer = ""
			return true
		}
	case "q", "ctrl+c":
		m.picker.close()
		return false
	}

	return true
}

func (m model) View() string {
	months := make([][]string, 0, 12)
	for month := time.January; month <= time.December; month++ {
		months = append(months, renderMonthLines(m.state.year, month, m.state, m.styles))
	}

	cols := m.columns()
	gap := strings.Repeat(" ", monthGapWidth)

	var b strings.Builder
	b.WriteString(renderControlBar(m.state, m.picker, m.styles))
	b.WriteString("\n")

	overlay := renderPickerOverlay(m, m.styles)
	if len(overlay) > 0 {
		for _, line := range overlay {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

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

	b.WriteString("\n\n")

	selected := fmt.Sprintf("Selected: %04d-%02d-%02d", m.state.year, int(m.state.month), m.state.day)
	b.WriteString(m.styles.footer.Render(selected))
	b.WriteString("\n")
	help := "Arrows/Vim: Move  n/p: Next/Prev month  N/P: Next/Prev year  Y/M: Pick year/month (type digits)  t: Today  q: Quit"
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

func renderControlBar(state calendarState, picker pickerState, styles styleSet) string {
	yearText := fmt.Sprintf("Year [%d]", state.year)
	monthText := fmt.Sprintf("Month [%s]", state.month.String())
	yearStyle := styles.control
	monthStyle := styles.control
	if picker.active == pickerYear {
		yearStyle = styles.controlActive
	}
	if picker.active == pickerMonth {
		monthStyle = styles.controlActive
	}
	parts := []string{
		yearStyle.Render(yearText),
		monthStyle.Render(monthText),
	}
	return strings.Join(parts, strings.Repeat(" ", 6))
}

func renderPickerOverlay(m model, styles styleSet) []string {
	switch m.picker.active {
	case pickerYear:
		lines := renderYearDropdown(m.picker.yearCursor, styles)
		return append(lines, styles.help.Render("Enter: Apply  Esc: Cancel  Up/Down: Navigate  Type digits"))
	case pickerMonth:
		lines := renderMonthDropdown(m.picker.monthCursor, styles)
		return append(lines, styles.help.Render("Enter: Apply  Esc: Cancel  Up/Down: Navigate  Type digits"))
	default:
		return nil
	}
}

func renderYearDropdown(cursor int, styles styleSet) []string {
	const visible = 9
	start := cursor - visible/2
	options := make([]int, visible)
	maxLen := 0
	for i := 0; i < visible; i++ {
		year := start + i
		options[i] = year
		if l := len(fmt.Sprintf("%d", year)); l > maxLen {
			maxLen = l
		}
	}
	lines := make([]string, 0, visible)
	for _, year := range options {
		label := fmt.Sprintf("%*d", maxLen, year)
		if year == cursor {
			lines = append(lines, styles.dropdownCursor.Render(label))
		} else {
			lines = append(lines, styles.dropdown.Render(label))
		}
	}
	return lines
}

func renderMonthDropdown(cursor int, styles styleSet) []string {
	lines := make([]string, 0, 12)
	maxLen := 0
	labels := make([]string, 12)
	for i := 0; i < 12; i++ {
		label := fmt.Sprintf("%2d %s", i+1, time.Month(i+1).String())
		labels[i] = label
		if len(label) > maxLen {
			maxLen = len(label)
		}
	}
	for i, label := range labels {
		padded := padRight(label, maxLen)
		if i == cursor {
			lines = append(lines, styles.dropdownCursor.Render(padded))
		} else {
			lines = append(lines, styles.dropdown.Render(padded))
		}
	}
	return lines
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}

func handleYearRunes(p *pickerState, runes []rune) {
	for _, r := range runes {
		if unicode.IsDigit(r) || (r == '-' && len(p.yearBuffer) == 0) {
			p.yearBuffer += string(r)
		}
	}
	if p.yearBuffer == "" {
		return
	}
	if val, err := strconv.Atoi(p.yearBuffer); err == nil {
		p.yearCursor = val
	}
}

func handleMonthRunes(p *pickerState, runes []rune) {
	for _, r := range runes {
		if !unicode.IsDigit(r) {
			continue
		}
		p.monthBuffer += string(r)
		if len(p.monthBuffer) > 2 {
			p.monthBuffer = p.monthBuffer[len(p.monthBuffer)-2:]
		}
	}
	if p.monthBuffer == "" {
		return
	}
	if val, err := strconv.Atoi(p.monthBuffer); err == nil {
		if val >= 1 && val <= 12 {
			p.monthCursor = val - 1
		}
	}
}

func trimLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
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
	control         lipgloss.Style
	controlActive   lipgloss.Style
	dropdown        lipgloss.Style
	dropdownCursor  lipgloss.Style
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
		control:         base.Copy().Foreground(lipgloss.Color("153")).Bold(true),
		controlActive:   base.Copy().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true),
		dropdown:        base.Copy().Padding(0, 1).Foreground(lipgloss.Color("252")),
		dropdownCursor:  base.Copy().Padding(0, 1).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true),
	}
}
