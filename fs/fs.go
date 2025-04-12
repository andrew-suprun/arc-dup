package fs

type FS interface {
	Run()
	Commands() chan<- any
	Events() <-chan any
}
