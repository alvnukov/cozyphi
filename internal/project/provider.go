package project

import "sync"

var (
	defaultProject *Project
	initOnce       sync.Once
)

// GetDefaultProject returns the singleton cozyphi workspace, discovered from the
// current working directory. It panics only if the global layout cannot be
// created (fs failure); configuration errors are surfaced by
// (*Project).LoadConfig so callers can report them without a stack trace.
func GetDefaultProject() *Project {
	initOnce.Do(func() {
		p, err := Discover("")
		if err != nil {
			panic(err)
		}
		defaultProject = p
	})
	return defaultProject
}
