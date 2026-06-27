// Package extdeps blank-imports the P5/P6 external SDKs so `go mod tidy` keeps
// them in go.mod while the media/sync/stats/export domains are implemented in
// parallel — the domain agents import these in real code WITHOUT having to touch
// go.mod (avoiding concurrent go.mod contention). This file is removed once P5/P6
// is fully wired and the real imports keep the deps pinned.
//
// These are go-1.24-compatible: asynq (go 1.22), excelize/v2 (go 1.18),
// cos-go-sdk-v5 (go 1.12), qcloud-cos-sts-sdk (go 1.x). See c5-backend-go124-dep-pinning.
package extdeps

import (
	_ "github.com/hibiken/asynq"
	_ "github.com/tencentyun/cos-go-sdk-v5"
	_ "github.com/tencentyun/qcloud-cos-sts-sdk/go"
	_ "github.com/xuri/excelize/v2"
)
