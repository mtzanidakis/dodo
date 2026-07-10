package api

import "time"

func loadTZ(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}
