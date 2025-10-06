# Calendar with Google Calendar Integration

A terminal-based calendar viewer with Google Calendar integration, built with Go and Bubbletea.

## Features

- 📅 View full year calendar in terminal
- 🔄 Sync with Google Calendar
- 📚 Support for multiple calendars
- 📍 View event details (time, location, description, calendar source)
- ⌨️ Keyboard navigation
- 🎨 Beautiful terminal UI with color highlighting
- • Event indicators on calendar dates
- ⚙️ Calendar selection and management

## Setup

### 1. Install Dependencies

```bash
go mod tidy
```

### 2. Setup Google Calendar API

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or select existing one)
3. Enable the **Google Calendar API**:
   - Navigate to "APIs & Services" > "Library"
   - Search for "Google Calendar API"
   - Click "Enable"
4. Create OAuth 2.0 credentials:
   - Go to "APIs & Services" > "Credentials"
   - Click "Create Credentials" > "OAuth client ID"
   - Select "Desktop app" as application type
   - Download the JSON file
5. Save the credentials file:
   - Create directory: `~/.config/calendar/`
   - Save the downloaded JSON as: `~/.config/calendar/credentials.json`

### 3. Build and Run

```bash
go build -o calendar
./calendar
```

On first run, you'll be prompted to authenticate:
1. A URL will be displayed in your terminal
2. Open the URL in your browser
3. Log in and authorize the application
4. Copy the authorization code from the browser
5. Paste it back in the terminal

The authentication token will be saved in `~/.config/calendar/token.json` for future use.

## Usage

### Keyboard Controls

**Navigation:**
- `←/→` or `h/l` - Move day left/right
- `↑/↓` or `k/j` - Move day up/down (by week)
- `n/p` - Next/Previous month
- `N/P` - Next/Previous year
- `t` - Jump to today

**Pickers:**
- `y/Y` - Open year picker (type digits to jump)
- `m/M` - Open month picker (type digits to select)

**Google Calendar:**
- `s/S` - Sync events from Google Calendar
- `e/E` - View events for selected day
- `c/C` - Manage calendars (select which calendars to sync)

**Other:**
- `q` or `Ctrl+C` - Quit
- `Esc` - Close picker/event view

### Event Indicators

Days with events are marked with a bullet point (•) before the day number on the calendar grid.

### Multiple Calendars

The application supports syncing from multiple Google Calendars:

1. Press `c` to open the calendar selection menu
2. Use `↑/↓` or `k/j` to navigate through your calendars
3. Press `Space` or `Enter` to toggle calendar selection
4. Press `a` to apply changes and sync events
5. Press `Esc` to cancel without changes

Selected calendars are saved in `~/.config/calendar/config.json` and will be used for all future syncs.

By default, only your primary calendar is synced. You can add work calendars, shared calendars, or any other calendars you have access to in your Google account.

## Troubleshooting

**"unable to read credentials file"**
- Ensure `credentials.json` is placed in `~/.config/calendar/`
- Check file permissions (should be readable)

**"unable to retrieve token from web"**
- Make sure you copied the full authorization code
- Try deleting `~/.config/calendar/token.json` and re-authenticating

**"Error syncing events"**
- Check your internet connection
- Verify the Google Calendar API is enabled in your project
- Try re-authenticating by deleting the token file

## File Structure

```
.
├── main.go              # Main application and UI logic
├── gcal/
│   ├── auth.go          # OAuth2 authentication
│   ├── events.go        # Google Calendar event fetching
│   └── config.go        # Calendar configuration management
├── cmd/logo/
│   └── main.go          # Logo generator
└── assets/
    └── calendar_logo.png
```

## Configuration Files

The application stores its configuration in `~/.config/calendar/`:
- `credentials.json` - Google OAuth2 credentials (you provide this)
- `token.json` - OAuth2 access token (auto-generated)
- `config.json` - Calendar selection preferences (auto-generated)
- `events_cache.json` - Cached events for fast startup (auto-generated)

### Event Caching

Events are automatically cached to disk for instant startup:
- Cache is valid for 24 hours
- On startup, cached events load immediately
- Background sync updates the cache if stale
- Cache clears when changing calendar selection
- Separate cache per year

## License

MIT