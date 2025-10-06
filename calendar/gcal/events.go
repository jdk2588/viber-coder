package gcal

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
)

type Event struct {
	ID           string
	Summary      string
	StartTime    time.Time
	EndTime      time.Time
	IsAllDay     bool
	Description  string
	Location     string
	CalendarID   string
	CalendarName string
	Color        string
}

func ListCalendars(srv *calendar.Service) ([]*calendar.CalendarListEntry, error) {
	calendarList, err := srv.CalendarList.List().Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve calendar list: %v", err)
	}
	return calendarList.Items, nil
}

func FetchEventsFromCalendar(srv *calendar.Service, calendarID, calendarName string, year int, month time.Month) (map[int][]Event, error) {
	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

	timeMin := startOfMonth.Format(time.RFC3339)
	timeMax := endOfMonth.Format(time.RFC3339)

	events, err := srv.Events.List(calendarID).
		ShowDeleted(false).
		SingleEvents(true).
		TimeMin(timeMin).
		TimeMax(timeMax).
		OrderBy("startTime").
		Do()

	if err != nil {
		return nil, fmt.Errorf("unable to retrieve events from %s: %v", calendarName, err)
	}

	eventsByDay := make(map[int][]Event)

	for _, item := range events.Items {
		event := Event{
			ID:           item.Id,
			Summary:      item.Summary,
			Description:  item.Description,
			Location:     item.Location,
			CalendarID:   calendarID,
			CalendarName: calendarName,
			Color:        item.ColorId,
		}

		var startTime time.Time
		var endTime time.Time

		if item.Start.DateTime != "" {
			startTime, err = time.Parse(time.RFC3339, item.Start.DateTime)
			if err != nil {
				continue
			}
			event.IsAllDay = false
		} else if item.Start.Date != "" {
			startTime, err = time.Parse("2006-01-02", item.Start.Date)
			if err != nil {
				continue
			}
			event.IsAllDay = true
		}

		if item.End.DateTime != "" {
			endTime, err = time.Parse(time.RFC3339, item.End.DateTime)
			if err != nil {
				continue
			}
		} else if item.End.Date != "" {
			endTime, err = time.Parse("2006-01-02", item.End.Date)
			if err != nil {
				continue
			}
		}

		event.StartTime = startTime
		event.EndTime = endTime

		day := startTime.Day()
		eventsByDay[day] = append(eventsByDay[day], event)
	}

	return eventsByDay, nil
}

func FetchEvents(srv *calendar.Service, calendarIDs []string, year int, month time.Month) (map[int][]Event, error) {
	allEventsByDay := make(map[int][]Event)

	for _, calID := range calendarIDs {
		cal, err := srv.Calendars.Get(calID).Do()
		calName := calID
		if err == nil && cal != nil {
			calName = cal.Summary
		}

		eventsByDay, err := FetchEventsFromCalendar(srv, calID, calName, year, month)
		if err != nil {
			continue
		}

		for day, events := range eventsByDay {
			allEventsByDay[day] = append(allEventsByDay[day], events...)
		}
	}

	return allEventsByDay, nil
}

func FetchAllMonthsEvents(srv *calendar.Service, calendarIDs []string, year int) (map[time.Month]map[int][]Event, error) {
	allEvents := make(map[time.Month]map[int][]Event)

	if srv == nil {
		return allEvents, fmt.Errorf("calendar service is nil")
	}

	if len(calendarIDs) == 0 {
		return allEvents, nil
	}

	startOfYear := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
	endOfYear := time.Date(year, time.December, 31, 23, 59, 59, 0, time.Local)

	timeMin := startOfYear.Format(time.RFC3339)
	timeMax := endOfYear.Format(time.RFC3339)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error

	for _, calID := range calendarIDs {
		cal, err := srv.Calendars.Get(calID).Context(ctx).Do()
		calName := calID
		if err == nil && cal != nil {
			calName = cal.Summary
		}

		events, err := srv.Events.List(calID).
			Context(ctx).
			ShowDeleted(false).
			SingleEvents(true).
			TimeMin(timeMin).
			TimeMax(timeMax).
			OrderBy("startTime").
			MaxResults(2500).
			Do()

		if err != nil {
			lastErr = err
			continue
		}

		for _, item := range events.Items {
			event := Event{
				ID:           item.Id,
				Summary:      item.Summary,
				Description:  item.Description,
				Location:     item.Location,
				CalendarID:   calID,
				CalendarName: calName,
				Color:        item.ColorId,
			}

			var startTime time.Time

			if item.Start.DateTime != "" {
				startTime, err = time.Parse(time.RFC3339, item.Start.DateTime)
				if err != nil {
					continue
				}
				event.IsAllDay = false
			} else if item.Start.Date != "" {
				startTime, err = time.Parse("2006-01-02", item.Start.Date)
				if err != nil {
					continue
				}
				event.IsAllDay = true
			}

			var endTime time.Time
			if item.End.DateTime != "" {
				endTime, err = time.Parse(time.RFC3339, item.End.DateTime)
				if err != nil {
					continue
				}
			} else if item.End.Date != "" {
				endTime, err = time.Parse("2006-01-02", item.End.Date)
				if err != nil {
					continue
				}
			}

			event.StartTime = startTime
			event.EndTime = endTime

			month := startTime.Month()
			day := startTime.Day()

			if allEvents[month] == nil {
				allEvents[month] = make(map[int][]Event)
			}
			allEvents[month][day] = append(allEvents[month][day], event)
		}
	}

	if len(allEvents) == 0 && lastErr != nil {
		return allEvents, lastErr
	}

	return allEvents, nil
}
