package model

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/codegangsta/martini-contrib/render"
	"github.com/heartbeatsjp/happo-agent/autoscaling"
	"github.com/heartbeatsjp/happo-agent/collect"
	"github.com/heartbeatsjp/happo-agent/db"
	"github.com/heartbeatsjp/happo-agent/halib"
	"github.com/heartbeatsjp/happo-agent/util"
)

var (
	// AppVersion equals main.Version
	AppVersion string

	startAt = time.Now()
)

// Status implements /status endpoint. returns status
func Status(req *http.Request, r render.Render) {
	log := util.HappoAgentLogger()

	bufSize := 4 * 1024 * 1024 // max 4MB
	buf := make([]byte, bufSize)
	bufferOverflow := false
	readBytes := runtime.Stack(buf, true)
	if readBytes < len(buf) {
		buf = buf[:readBytes] // shrink
	} else {
		// note: strictly saying, in case stack is just same as bufSize, buffer is not overflow.
		bufferOverflow = true
	}
	callers := strings.Split(string(buf), "\n\n")
	if bufferOverflow {
		callers = append(callers, "...")
	}
	log.Debugf("callers: %v", callers)

	propertiesName := []string{
		"leveldb.num-files-at-level0",
		"leveldb.num-files-at-level1",
		"leveldb.num-files-at-level2",
		"leveldb.stats",
		"leveldb.writedelay",
		"leveldb.sstables",
		"leveldb.blockpool",
		"leveldb.cachedblock",
		"leveldb.openedtables",
		"leveldb.alivesnaps",
		"leveldb.aliveiters",
	}
	leveldbProperties := map[string]string{}
	for _, propertyName := range propertiesName {
		propertyValue, _ := db.DB.GetProperty(propertyName)
		leveldbProperties[propertyName] = propertyValue
	}

	log.Debugf("leveldbProperties:%v", leveldbProperties)

	statusResponse := &halib.StatusResponse{
		AppVersion:         AppVersion,
		UptimeSeconds:      int64(time.Since(startAt) / time.Second),
		NumGoroutine:       runtime.NumGoroutine(),
		MetricBufferStatus: collect.GetMetricDataBufferStatus(false),
		Callers:            callers,
		LevelDBProperties:  leveldbProperties,
	}
	r.JSON(http.StatusOK, statusResponse)
}

// RequestStatus implements /status/request endpoint. returns status
func RequestStatus(req *http.Request, r render.Render) {
	requestStatus := util.GetMartiniRequestStatus(time.Now())

	r.JSON(http.StatusOK, requestStatus)
}

// MemoryStatus implements /status/memory endpoint. returns runtime.Memstatus in JSON
func MemoryStatus(req *http.Request, r render.Render) {
	mem := new(runtime.MemStats)
	runtime.ReadMemStats(mem)
	r.JSON(http.StatusOK, mem)
}

// AutoScalingStatus implements /status/autoscaling endpoint
func AutoScalingStatus(req *http.Request, r render.Render) {
	config, err := autoscaling.GetAutoScalingConfig(AutoScalingConfigFile)
	if err != nil {
		r.JSON(http.StatusInternalServerError, halib.AutoScalingStatus{})
		return
	}

	client := autoscaling.NewAWSClient()
	var status []halib.AutoScalingStatus
	for _, a := range config.AutoScalings {
		diff, err := autoscaling.CompareInstances(client, a.AutoScalingGroupName, a.HostPrefix)
		if err != nil {
			status = append(status, halib.AutoScalingStatus{
				AutoScalingGroupName: a.AutoScalingGroupName,
				Status:               "error",
				Message:              "check failed",
			})
			continue
		}
		if len(diff) > 0 {
			status = append(status, halib.AutoScalingStatus{
				AutoScalingGroupName: a.AutoScalingGroupName,
				Status:               "error",
				Message:              fmt.Sprintf("difference found: %s", strings.Join(diff, ",")),
			})
			continue
		}
		status = append(status, halib.AutoScalingStatus{
			AutoScalingGroupName: a.AutoScalingGroupName,
			Status:               "ok",
			Message:              "",
		})
	}

	r.JSON(http.StatusOK, status)
}
