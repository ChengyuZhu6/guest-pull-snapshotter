package version

var (
	// Version holds the complete version number. Filled in at linking time.
	Version = "unknown"

	// Revision is filled with the VCS (e.g. git) revision being used to build
	// the program at linking time.
	Revision = "unknown"

	// BuildTimestamp is timestamp of building.
	BuildTimestamp = "unknown"
)
