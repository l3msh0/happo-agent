package util

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/heartbeatsjp/happo-agent/halib"

	"github.com/Songmu/timeout"
	"github.com/codegangsta/cli"
)

// Global Variables

// CommandTimeout is command execution timeout sec
var CommandTimeout time.Duration = -1

// Production is flag. when production use, set true
var Production bool

// TimeoutError is error struct show error is timeout
type TimeoutError struct {
	Message string
}

func (err *TimeoutError) Error() string {
	return err.Message
}

// --- Function
func init() {
	Production = strings.ToLower(os.Getenv("MARTINI_ENV")) == "production"
}

// ExecCommand execute command with specified timeout behavior
func ExecCommand(command string, option string) (int, string, string, error) {
	var timeBegin time.Time
	var cswBegin int
	if HappoAgentLoggerEnableInfo() {
		timeBegin = time.Now()
		cswBegin = getContextSwitch()
	}

	commandTimeout := CommandTimeout
	if commandTimeout == -1 {
		commandTimeout = halib.DefaultCommandTimeout
	}

	commandWithOptions := fmt.Sprintf("%s %s", command, option)
	tio := &timeout.Timeout{
		Cmd:       exec.Command("/bin/sh", "-c", commandWithOptions),
		Duration:  commandTimeout * time.Second,
		KillAfter: halib.CommandKillAfterSeconds * time.Second,
	}
	if runtime.GOOS == "windows" {
		// Force last command's exit code to be PowerShell's exit code
		commandWithOptions += "; exit $LastExitCode"
		tio = &timeout.Timeout{
			Cmd:       exec.Command("powershell.exe", commandWithOptions),
			Duration:  commandTimeout * time.Second,
			KillAfter: halib.CommandKillAfterSeconds * time.Second,
		}
	}
	exitStatus, stdout, stderr, err := tio.Run()

	if err == nil && exitStatus.IsTimedOut() {
		err = &TimeoutError{"Exec timeout: " + commandWithOptions}
	}

	if HappoAgentLoggerEnableInfo() {
		now := time.Now()
		cswTook := getContextSwitch() - cswBegin
		timeTook := now.Sub(timeBegin)
		HappoAgentLogger().Infof("%v: ExecCommand %v end. csw=%v, duration=%v,", now.Format(time.RFC3339Nano), command, cswTook, timeTook.Seconds())
	}
	return exitStatus.GetChildExitCode(), stdout, stderr, err
}

func getContextSwitch() int {
	if _, err := os.Stat("/proc/stat"); err != nil {
		return -1
	}
	fp, err := os.Open("/proc/stat")
	defer fp.Close()
	if err != nil {
		return -1
	}
	for scanner := bufio.NewScanner(fp); scanner.Scan(); {
		line := scanner.Text()
		if strings.HasPrefix(line, "ctxt ") {
			csw, err := strconv.Atoi(strings.Split(line, " ")[1])
			if err != nil {
				return -1
			}
			return csw
		}
	}
	return -1
}

// ExecCommandCombinedOutput execute command with specified timeout behavior
func ExecCommandCombinedOutput(command string, option string) (int, string, error) {

	commandTimeout := CommandTimeout
	if commandTimeout == -1 {
		commandTimeout = halib.DefaultCommandTimeout
	}

	commandWithOptions := fmt.Sprintf("%s %s", command, option)
	tio := &timeout.Timeout{
		Cmd:       exec.Command("/bin/sh", "-c", commandWithOptions),
		Duration:  commandTimeout * time.Second,
		KillAfter: halib.CommandKillAfterSeconds * time.Second,
	}
	if runtime.GOOS == "windows" {
		tio = &timeout.Timeout{
			Cmd:       exec.Command("powershell.exe", commandWithOptions),
			Duration:  commandTimeout * time.Second,
			KillAfter: halib.CommandKillAfterSeconds * time.Second,
		}
	}
	out := &bytes.Buffer{}
	tio.Cmd.Stdout = out
	tio.Cmd.Stderr = out

	ch, err := tio.RunCommand()
	exitStatus := <-ch

	if err == nil && exitStatus.IsTimedOut() {
		err = &TimeoutError{"Exec timeout: " + commandWithOptions}
	}

	return exitStatus.GetChildExitCode(), out.String(), err

}

// BindManageParameter build and return ManageRequest
func BindManageParameter(c *cli.Context) (halib.ManageRequest, error) {
	var hostinfo halib.CrawlConfigAgent
	var manageRequest halib.ManageRequest

	hostinfo.GroupName = c.String("group_name")
	if hostinfo.GroupName == "" {
		return manageRequest, errors.New("group_name is null")
	}
	hostinfo.Port = c.Int("port")

	if c.Command.Name == "add_ag" {
		hostinfo.IP = c.String("autoscaling_group_name")
		if hostinfo.IP == "" {
			return manageRequest, errors.New("autoscaling_group_name is null")
		}
		hostinfo.Hostname = c.String("autoscaling_group_name")
		hostinfo.AutoScaling.AutoScalingGroupName = c.String("autoscaling_group_name")

		hostinfo.Proxies = c.StringSlice("proxy")
		if len(hostinfo.Proxies) < 1 {
			return manageRequest, errors.New("proxy is null")
		}

		hostinfo.AutoScaling.AutoScalingCount = c.Int("autoscaling_count")
		if hostinfo.AutoScaling.AutoScalingCount < 1 {
			return manageRequest, errors.New("autoscaling_count is lower than 1")
		}
		hostinfo.AutoScaling.HostPrefix = c.String("host_prefix")
		if hostinfo.AutoScaling.HostPrefix == "" {
			return manageRequest, errors.New("host_prefix is null")
		}
	} else {
		hostinfo.IP = c.String("ip")
		if hostinfo.IP == "" {
			return manageRequest, errors.New("ip is null")
		}
		hostinfo.Hostname = c.String("hostname")
		hostinfo.Proxies = c.StringSlice("proxy")
	}

	manageRequest.Hostdata = hostinfo

	return manageRequest, nil
}

// RequestToManageAPI send request to ManageAPI
func RequestToManageAPI(endpoint string, path string, postdata []byte) (*http.Response, error) {
	uri := fmt.Sprintf("%s%s", endpoint, path)
	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(postdata))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultTransport.RoundTrip(req)
}

// RequestToMetricAppendAPI send request to MetricAppendPI
func RequestToMetricAppendAPI(endpoint string, postdata []byte) (*http.Response, error) {
	client, req, err := buildMetricAppendAPIRequest(endpoint, postdata)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func buildMetricAppendAPIRequest(endpoint string, postdata []byte) (*http.Client, *http.Request, error) {
	uri := fmt.Sprintf("%s/metric/append", endpoint)
	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(postdata))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	//FIXME other parameters should be proper values
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client, req, err
}

// RequestToAutoScalingAPI send request to AutoScalingAPI
func RequestToAutoScalingAPI(endpoint string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/autoscaling", endpoint)
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	return client.Do(req)
}

// RequestToCheckAvailableAPI send request to AutoScalingHealthAPI
func RequestToCheckAvailableAPI(endpoint string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/", endpoint)
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req = req.WithContext(ctx)

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	return client.Do(req)
}

// RequestToAutoScalingResolveAPI send request to AutoScalingResolveAPI
func RequestToAutoScalingResolveAPI(endpoint string, alias string) (*http.Response, error) {
	client, req, err := buildAutoScalingResolveAPIRequest(endpoint, alias)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func buildAutoScalingResolveAPIRequest(endpoint string, alias string) (*http.Client, *http.Request, error) {
	uri := fmt.Sprintf("%s/autoscaling/resolve/%s", endpoint, alias)
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client, req, err
}

// RequestToAutoScalingInstanceAPI send request to AutoScalingInstanceAPI
func RequestToAutoScalingInstanceAPI(endpoint, requestType string, postdata []byte) (*http.Response, error) {
	client, req, err := buildAutoScalingRegisterInstanceAPIRequest(endpoint, requestType, postdata)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func buildAutoScalingRegisterInstanceAPIRequest(endpoint, requestType string, postdata []byte) (*http.Client, *http.Request, error) {
	uri := fmt.Sprintf("%s/autoscaling/instance/%s", endpoint, requestType)
	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(postdata))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client, req, err
}

// RequestToAutoScalingLeaveAPI send request to AutoScalingInstanceAPI
func RequestToAutoScalingLeaveAPI(endpoint string, postdata []byte) (*http.Response, error) {
	client, req, err := buildAutoScalingLeaveAPIRequest(endpoint, postdata)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func buildAutoScalingLeaveAPIRequest(endpoint string, postdata []byte) (*http.Client, *http.Request, error) {
	uri := fmt.Sprintf("%s/autoscaling/leave", endpoint)
	req, err := http.NewRequest("POST", uri, bytes.NewBuffer(postdata))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client, req, err
}
