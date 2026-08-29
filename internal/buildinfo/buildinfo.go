package buildinfo

// Version is a variable so release builds can stamp the exact package version
// with -ldflags without changing source files. Development builds keep the
// checked-in fallback below.
var Version = "2.2.0-memory-harness"
