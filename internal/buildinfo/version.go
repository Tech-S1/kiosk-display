package buildinfo

var (
	version = "0.0.0"
	commit  = "none"
	date    = "unknown"
)

func Version() string { return version }

func Commit() string { return commit }

func Date() string { return date }
