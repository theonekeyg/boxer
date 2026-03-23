package api

import "boxer/config"

// RunRequest is the JSON body for POST /run.
type RunRequest struct {
	Image   string                 `json:"image"   binding:"required"       example:"python:3.12-slim"`
	Cmd     []string               `json:"cmd"     binding:"required,min=1" example:"python3,-c,print('hello world')"`
	Env     []string               `json:"env"     example:"HOME=/root,PYTHONPATH=/app"`
	Cwd     string                 `json:"cwd"     example:"/app"`
	Limits  *config.ResourceLimits `json:"limits"`
	// Files lists relative paths of files previously uploaded via POST /files.
	// Each file is bind-mounted read-only at /path inside the container.
	Files   []string `json:"files"   example:"workspace/script.py,workspace/data.json"`
	// Persist keeps uploaded input files and captured output files after the run.
	// Default false: all files are deleted once the response is returned.
	Persist bool `json:"persist"`
	// Network sets the container network mode: none (default, no access),
	// sandbox (isolated NAT namespace), or host (shared host network).
	Network string `json:"network" enums:"none,sandbox,host" default:"none" example:"none"`
}

// RunResponse is the JSON body returned for a completed execution.
type RunResponse struct {
	ExecID   string `json:"exec_id"   example:"boxer-abc123"`
	ExitCode int    `json:"exit_code" example:"0"`
	Stdout   string `json:"stdout"    example:"hello world\n"`
	Stderr   string `json:"stderr"    example:""`
	WallMs   int64  `json:"wall_ms"   example:"342"`
}

// UploadResponse is returned by POST /files on success.
type UploadResponse struct {
	Path string `json:"path" example:"workspace/script.py"`
}

// ErrorResponse is returned on all non-200 responses.
type ErrorResponse struct {
	Error string `json:"error" example:"image pull failed: not found"`
}
