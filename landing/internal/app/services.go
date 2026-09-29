package app

// Service describes one linked application: reachable directly on the
// docker network at Host:Port for server-side health checks, and on the
// tailnet at the same Port via ListenDirectPorts.
//
// IconPage is fetched directly at Host:Port so /api/icon/<key> can scrape
// its real <link rel="icon"> instead of hardcoding a filename that changes
// across releases.
type Service struct {
	Key       string
	Name      string
	Host      string
	Port      int
	CheckPath string
	IconPage  string
}

// Services monitored on the dashboard, each reachable directly at
// <hostname>:<Port> on the tailnet.
var linkedServices = []Service{
	{
		Key:       "jellyfin",
		Name:      "Jellyfin",
		Host:      "jellyfin",
		Port:      8096,
		CheckPath: "/health",
		IconPage:  "/jellyfin/web/",
	},
	{
		Key:       "navidrome",
		Name:      "Navidrome",
		Host:      "navidrome",
		Port:      4533,
		CheckPath: "/ping",
		IconPage:  "/navidrome/app/",
	},
	{
		Key:       "filebrowser",
		Name:      "File Browser",
		Host:      "filebrowser",
		Port:      8080,
		CheckPath: "/health",
		IconPage:  "/filebrowser/",
	},
	{
		Key:       "immich",
		Name:      "Immich",
		Host:      "immich",
		Port:      2283,
		CheckPath: "/api/health",
		IconPage:  "/",
	},
}
