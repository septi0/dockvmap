package model

import "time"

type WorkerTick struct {
	Job       string
	LastRunAt time.Time
	LastError string
	LastCount *int64
}
