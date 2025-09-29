package version

import "fmt"

var (
	// Version is the version of the application
	Version = "dev"

	// Commit is the commit hash of the application
	Commit = "none"

	// Date is the date of the build
	Date = "unknown"
)

func GetVersion() string {
	return fmt.Sprintf("Version: %s, Commit: %s, Date: %s\n", Version, Commit, Date)
}
