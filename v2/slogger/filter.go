package slogger

// enables level-filtering before a Log entry is created, avoiding the runtime.Caller invocation
// return true if filter evaluation should continue
type TurboFilter func(level Level, messageFmt string, args ...interface{}) bool

func TurboLevelFilter(threshold Level) func(Level, string, ...interface{}) bool {
	return func(level Level, messageFmt string, args ...interface{}) bool {
		return level >= threshold
	}
}
