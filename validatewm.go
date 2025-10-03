package cleanmw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
)

type ExecRequest struct {
	// 方式一：cmd + args
	Cmd       string   `json:"cmd"`        // 如: "ls"
	Args      []string `json:"args"`       // 如: ["-l","/usr"]
	// 方式二：整条命令（将由 bash -lc 执行，支持管道/重定向）
	Shell     string   `json:"shell"`      // 如: "ls -l /usr | grep bin"
	TimeoutSec int     `json:"timeoutSec"` // 可选，默认 10 秒
}

type ExecResponse struct {
	Code        int    `json:"code"`         // 0: 成功，其它: 失败
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exitCode"`
	DurationMS  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
}

// Exec 返回一个可直接挂载的 Gin 处理器：POST JSON -> 执行命令 -> 返回结果
func Exec() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ExecRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ExecResponse{Code: 1, Error: "bad request: " + err.Error()})
			return
		}
		if req.TimeoutSec <= 0 {
			req.TimeoutSec = 10
		}

		start := time.Now()
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if req.Shell != "" {
			// bash -lc 方式，支持管道/通配/重定向
			cmd = exec.CommandContext(ctx, "/bin/bash", "-lc", req.Shell)
		} else if req.Cmd != "" {
			// 直接 cmd + args
			cmd = exec.CommandContext(ctx, req.Cmd, req.Args...)
		} else {
			c.JSON(http.StatusBadRequest, ExecResponse{Code: 2, Error: "cmd or shell must be provided"})
			return
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		duration := time.Since(start)

		exitCode := 0
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else if ctx.Err() == context.DeadlineExceeded {
				exitCode = 124 // 超时惯例
			} else {
				exitCode = 1
			}
		}

		resp := ExecResponse{
			Code:       0,
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			ExitCode:   exitCode,
			DurationMS: duration.Milliseconds(),
		}
		if runErr != nil {
			resp.Code = 3
			resp.Error = runErr.Error()
		}

		c.Header("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(c.Writer)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(resp)
	}
}
