package zv

import (
	"time"
)

type Zip struct {
	Files   []string
	Fetched time.Time
	Expires time.Time
}

type File struct {
	Name     string
	Content  []byte
	TooLarge bool
}
