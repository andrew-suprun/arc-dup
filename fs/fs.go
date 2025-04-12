package fs

type Events interface {
	Send(msg any)
}

type EventDebugScan struct {
	Idx int
	N   int
}

type FS interface {
	Scan(events Events)
	Sync(commands []any, events Events)
}
