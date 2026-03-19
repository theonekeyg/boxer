package api

import "boxer/config"

// RunRequest is the JSON body for POST /run.
type RunRequest struct {
	Image   string                 `json:"image"   binding:"required"  example:"python:3.12-slim"`
	Cmd     []string               `json:"cmd"     binding:"required,min=1" example:"python -c 'print(\"hello world\")'"`
	Env     []string               `json:"env"     example:"HOME=/root"`
	Cwd     string                 `json:"cwd"     example:"/app"`
	Limits  *config.ResourceLimits `json:"limits"`
	Files   []string               `json:"files"`   // relative paths of uploaded files to bind-mount read-only
	Persist bool                   `json:"persist"` // keep input files after run (default false)
}

// RunResponse is the JSON body returned for a completed execution.
type RunResponse struct {
	ExecID   string `json:"exec_id"   example:"boxer-abc123"`
	ExitCode int    `json:"exit_code" example:"0"`
	Stdout   string `json:"stdout"    example:"hello world\n"`
	Stderr   string `json:"stderr"    example:""`
	WallMs   int64  `json:"wall_ms"   example:"342"`
}

// ErrorResponse is returned on all non-200 responses.
type ErrorResponse struct {
	Error string `json:"error" example:"image pull failed: not found"`
}
