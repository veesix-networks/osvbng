package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Full() string {
	return Version + " (" + Commit + ") built on " + Date
}
