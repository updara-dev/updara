package model

import "time"

const (
	CmdStatusPending = "pending"
	CmdStatusRunning = "running"
	CmdStatusDone    = "done"
	CmdStatusFailed  = "failed"
)

type Command struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	Connector string    `json:"connector"`
	Cmd       string    `json:"cmd"`
	Status    string    `json:"status"`
	Output    string    `json:"output"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
