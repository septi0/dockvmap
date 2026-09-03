package service

import "time"

type WorkerJobDescriptor struct {
	Name           string
	Description    string
	Interval       time.Duration
	Enabled        bool
	DisabledReason string
	Triggerable    bool
}

type WorkerCatalog struct {
	jobs []WorkerJobDescriptor
}

func NewWorkerCatalog(jobs []WorkerJobDescriptor) *WorkerCatalog {
	return &WorkerCatalog{jobs: jobs}
}

func (c *WorkerCatalog) Jobs() []WorkerJobDescriptor {
	return c.jobs
}
