package api

import "boxer/config"

// RunRequest is the JSON body for POST /run.
type RunRequest struct {
	Image  string                 `json:"image"  binding:"required"`
	Cmd    []string               `json:"cmd"    binding:"required,min=1"`
	Env    []string               `json:"env"`
	Cwd    string                 `json:"cwd"`
	Limits *config.ResourceLimits `json:"limits"`
}

// RunResponse is the JSON body returned for a completed execution.
type RunResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	WallMs   int64  `json:"wall_ms"`
}
