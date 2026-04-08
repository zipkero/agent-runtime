package state

type AgentStatus string

const (
	StatusRunning      AgentStatus = "running"
	StatusFinished     AgentStatus = "finished"
	StatusFailed       AgentStatus = "failed"
	StatusWaitingInput AgentStatus = "waiting_input"
)
