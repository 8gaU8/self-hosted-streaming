package app

// Service describes one linked application: it has a web UI reverse-proxied
// by landing at Link, and is reachable directly on the docker network at
// Host:Port for server-side health checks and proxying.
//
// IconPage is the same app fetched directly (bypassing the /<key>/ proxy
// prefix) so /api/icon/<key> can scrape its real <link rel="icon"> instead
// of hardcoding a filename that changes across releases.
type Service struct {
	Key       string
	Name      string
	Host      string
	Port      int
	CheckPath string
	Link      string
	IconPage  string
}

// Services with a web UI, reverse-proxied by landing and monitored on the
// dashboard.
var linkedServices = []Service{
	{
		Key:       "jellyfin",
		Name:      "Jellyfin",
		Host:      "jellyfin",
		Port:      8096,
		CheckPath: "/health",
		Link:      "/jellyfin/",
		IconPage:  "/jellyfin/web/",
	},
	{
		Key:       "navidrome",
		Name:      "Navidrome",
		Host:      "navidrome",
		Port:      4533,
		CheckPath: "/ping",
		Link:      "/navidrome/",
		IconPage:  "/navidrome/app/",
	},
	{
		Key:       "filebrowser",
		Name:      "File Browser",
		Host:      "filebrowser",
		Port:      8080,
		CheckPath: "/health",
		Link:      "/filebrowser/",
		IconPage:  "/filebrowser/",
	},
}

