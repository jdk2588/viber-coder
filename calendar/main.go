package main

import (
	"calendar/gcal"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/api/calendar/v3"
)

type calendarState struct {
	year   int
	month  time.Month
	day    int
	events map[time.Month]map[int][]gcal.Event
}

type model struct {
	styles       styleSet
	state        calendarState
	width        int
	picker       pickerState
	calendarSrv  *calendar.Service
	eventView    eventViewState
	calendarView calendarViewState
	syncing      bool
	syncError    string
	config       *gcal.Config
}

type eventViewState struct {
	active bool
	events []gcal.Event
}

type calendarViewState struct {
	active    bool
	calendars []*calendar.CalendarListEntry
	selected  map[string]bool
	cursor    int
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

	config, _ := gcal.LoadConfig()
	if config == nil {
		config = &gcal.Config{CalendarIDs: []string{"primary"}}
	}

	cachedEvents, isFresh := gcal.LoadEventsCache(now.Year())
	if cachedEvents == nil {
		cachedEvents = make(map[time.Month]map[int][]gcal.Event)
	}

	srv, err := gcal.GetCalendarService()
	syncErr := ""

	if err != nil {
		syncErr = err.Error()
	}

	shouldSync := srv != nil && err == nil && !isFresh

	m := model{
		styles:      newStyles(),
		calendarSrv: srv,
		syncError:   syncErr,
		config:      config,
		syncing:     shouldSync,
		state: calendarState{
			year:   now.Year(),
			month:  now.Month(),
			day:    now.Day(),
			events: cachedEvents,
		},
	}

	return m
}

func (m model) Init() tea.Cmd {
	if m.calendarSrv != nil && m.syncing {
		return syncEvents(m.calendarSrv, m.config.CalendarIDs, m.state.year)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if m.eventView.active {
			if msg.String() == "esc" || msg.String() == "e" || msg.String() == "q" {
				m.eventView.active = false
			}
			return m, nil
		}

		if m.calendarView.active {
			if handleCalendarViewInput(&m, msg) {
				return m, nil
			}
		}

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
			if m.state.year != time.Now().Year() {
				cachedEvents, _ := gcal.LoadEventsCache(m.state.year)
				if cachedEvents != nil {
					m.state.events = cachedEvents
				} else {
					m.state.events = make(map[time.Month]map[int][]gcal.Event)
					if m.calendarSrv != nil && !m.syncing {
						m.syncing = true
						return m, syncEvents(m.calendarSrv, m.config.CalendarIDs, m.state.year)
					}
				}
			}
		case "P":
			adjustYear(&m.state, -1)
			if m.state.year != time.Now().Year() {
				cachedEvents, _ := gcal.LoadEventsCache(m.state.year)
				if cachedEvents != nil {
					m.state.events = cachedEvents
				} else {
					m.state.events = make(map[time.Month]map[int][]gcal.Event)
					if m.calendarSrv != nil && !m.syncing {
						m.syncing = true
						return m, syncEvents(m.calendarSrv, m.config.CalendarIDs, m.state.year)
					}
				}
			}
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
		case "s", "S":
			if m.calendarSrv != nil && !m.syncing {
				m.syncing = true
				return m, syncEvents(m.calendarSrv, m.config.CalendarIDs, m.state.year)
			}
		case "c", "C":
			if m.calendarSrv != nil && !m.calendarView.active {
				calendars, err := gcal.ListCalendars(m.calendarSrv)
				if err == nil {
					m.calendarView.active = true
					m.calendarView.calendars = calendars
					m.calendarView.selected = make(map[string]bool)
					for _, id := range m.config.CalendarIDs {
						m.calendarView.selected[id] = true
					}
					m.calendarView.cursor = 0
				}
			}
		case "e", "E":
			if events, ok := m.state.events[m.state.month]; ok {
				if dayEvents, ok := events[m.state.day]; ok && len(dayEvents) > 0 {
					m.eventView.active = true
					m.eventView.events = dayEvents
				}
			}
		}
	case syncEventsMsg:
		m.syncing = false
		if msg.err != nil {
			m.syncError = fmt.Sprintf("Sync failed: %v", msg.err)
		} else {
			if msg.events != nil {
				m.state.events = msg.events
				gcal.SaveEventsCache(m.state.year, msg.events)
			}
			m.syncError = ""
		}
		return m, nil
	}
	clampDay(&m.state)
	return m, nil
}

type syncEventsMsg struct {
	events map[time.Month]map[int][]gcal.Event
	err    error
}

func syncEvents(srv *calendar.Service, calendarIDs []string, year int) tea.Cmd {
	return func() tea.Msg {
		if srv == nil {
			return syncEventsMsg{events: nil, err: fmt.Errorf("calendar service not initialized")}
		}
		if len(calendarIDs) == 0 {
			return syncEventsMsg{events: make(map[time.Month]map[int][]gcal.Event), err: nil}
		}

		events, err := gcal.FetchAllMonthsEvents(srv, calendarIDs, year)
		if err != nil {
			return syncEventsMsg{events: make(map[time.Month]map[int][]gcal.Event), err: err}
		}
		return syncEventsMsg{events: events, err: nil}
	}
}

func handleCalendarViewInput(m *model, msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc", "q", "c", "C":
		m.calendarView.active = false
		return true
	case "up", "k":
		if m.calendarView.cursor > 0 {
			m.calendarView.cursor--
		}
		return true
	case "down", "j":
		if m.calendarView.cursor < len(m.calendarView.calendars)-1 {
			m.calendarView.cursor++
		}
		return true
	case " ", "enter":
		if m.calendarView.cursor < len(m.calendarView.calendars) {
			calID := m.calendarView.calendars[m.calendarView.cursor].Id
			m.calendarView.selected[calID] = !m.calendarView.selected[calID]
		}
		return true
	case "a", "A":
		newIDs := []string{}
		for id, selected := range m.calendarView.selected {
			if selected {
				newIDs = append(newIDs, id)
			}
		}
		if len(newIDs) == 0 {
			newIDs = []string{"primary"}
		}
		m.config.CalendarIDs = newIDs
		gcal.SaveConfig(m.config)
		gcal.ClearEventsCache()
		m.calendarView.active = false
		if m.calendarSrv != nil && !m.syncing {
			m.syncing = true
			return true
		}
		return true
	}
	return false
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
	if m.eventView.active {
		return renderEventView(m.eventView.events, m.state, m.styles)
	}

	if m.calendarView.active {
		return renderCalendarView(m.calendarView, m.styles)
	}

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

	help := "Arrows/Vim: Move  n/p: Next/Prev month  N/P: Next/Prev year  Y/M: Pick year/month  t: Today  e: View events  s: Sync  c: Calendars  q: Quit"
	b.WriteString(m.styles.help.Render(help))

	if m.syncing {
		b.WriteString("\n")
		b.WriteString(m.styles.controlActive.Render(" ⟳ Syncing with Google Calendar... "))
	}

	if m.syncError != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.help.Render(fmt.Sprintf("Error: %s", m.syncError)))
	}

	return b.String()
}

func renderEventView(events []gcal.Event, state calendarState, styles styleSet) string {
	var b strings.Builder

	title := fmt.Sprintf("Events for %04d-%02d-%02d", state.year, int(state.month), state.day)
	b.WriteString(styles.header.Render(title))
	b.WriteString("\n\n")

	if len(events) == 0 {
		b.WriteString(styles.help.Render("No events for this day"))
	} else {
		for i, event := range events {
			b.WriteString(styles.selectedDay.Render(fmt.Sprintf("  %s  ", event.Summary)))
			b.WriteString("\n")

			if event.CalendarName != "" && event.CalendarName != event.CalendarID {
				b.WriteString(styles.help.Render(fmt.Sprintf("  📅 %s", event.CalendarName)))
				b.WriteString("\n")
			}

			if event.IsAllDay {
				b.WriteString(styles.help.Render("  All day"))
			} else {
				timeStr := fmt.Sprintf("  %s - %s",
					event.StartTime.Format("15:04"),
					event.EndTime.Format("15:04"))
				b.WriteString(styles.help.Render(timeStr))
			}
			b.WriteString("\n")

			if event.Location != "" {
				b.WriteString(styles.help.Render(fmt.Sprintf("  📍 %s", event.Location)))
				b.WriteString("\n")
			}

			if event.Description != "" {
				desc := event.Description
				if len(desc) > 100 {
					desc = desc[:100] + "..."
				}
				b.WriteString(styles.help.Render(fmt.Sprintf("  %s", desc)))
				b.WriteString("\n")
			}

			if i < len(events)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styles.help.Render("Press 'e' or 'esc' to return"))

	return b.String()
}

func renderCalendarView(view calendarViewState, styles styleSet) string {
	var b strings.Builder

	b.WriteString(styles.header.Render("Select Calendars"))
	b.WriteString("\n\n")
	b.WriteString(styles.help.Render("Use ↑/↓ to navigate, Space to toggle, 'a' to apply, 'esc' to cancel"))
	b.WriteString("\n\n")

	for i, cal := range view.calendars {
		checkbox := "[ ]"
		if view.selected[cal.Id] {
			checkbox = "[✓]"
		}

		label := fmt.Sprintf("%s %s", checkbox, cal.Summary)

		if i == view.cursor {
			b.WriteString(styles.selectedDay.Render(fmt.Sprintf("  %s  ", label)))
		} else {
			b.WriteString(styles.help.Render(fmt.Sprintf("  %s", label)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.help.Render("Press 'a' to apply changes, 'esc' to cancel"))

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

	eventsForMonth := state.events[month]

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
				hasEvents := len(eventsForMonth[currentDay]) > 0
				text = fmt.Sprintf("%2d", currentDay)

				isWeekend := weekday == 0 || weekday == 6

				if currentDay == state.day && month == state.month && year == state.year {
					if isWeekend {
						style = styles.selectedWeekend
					} else {
						style = styles.selectedDay
					}
				} else if hasEvents {
					if isWeekend {
						style = styles.eventWeekend
					} else {
						style = styles.eventDay
					}
				} else if isWeekend {
					style = styles.weekend
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
	eventDay        lipgloss.Style
	eventWeekend    lipgloss.Style
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
		eventDay:        base.Copy().Foreground(lipgloss.Color("51")).Bold(true),
		eventWeekend:    base.Copy().Foreground(lipgloss.Color("205")).Bold(true),
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
