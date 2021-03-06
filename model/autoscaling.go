package model

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/codegangsta/martini-contrib/render"
	"github.com/go-martini/martini"
	"github.com/heartbeatsjp/happo-agent/autoscaling"
	"github.com/heartbeatsjp/happo-agent/halib"
	"github.com/heartbeatsjp/happo-agent/util"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// AutoScalingConfigFile is filepath of autoscaling config file
	AutoScalingConfigFile string
	// AutoScalingBastionEndpoint is endpoint of autoscaling bastion when using running with autoscaling node
	AutoScalingBastionEndpoint string
)

// AutoScaling list autoscaling instances
func AutoScaling(req *http.Request, r render.Render) {
	var autoScalingResponse halib.AutoScalingResponse

	autoScaling, err := autoscaling.AutoScaling(AutoScalingConfigFile)
	if err != nil {
		r.JSON(http.StatusInternalServerError, autoScalingResponse)
		return
	}
	autoScalingResponse.AutoScaling = autoScaling
	r.JSON(http.StatusOK, autoScalingResponse)
}

// AutoScalingResolve return ip of alias
func AutoScalingResolve(params martini.Params, r render.Render) {
	var response halib.AutoScalingResolveResponse
	alias := params["alias"]
	if alias == "" {
		response.Status = "error"
		r.JSON(http.StatusBadRequest, response)
		return
	}

	ip, err := autoscaling.AliasToIP(alias)
	if err != nil {
		response.Status = "error"
		r.JSON(http.StatusInternalServerError, response)
		return
	}
	response.Status = "OK"
	response.IP = ip

	r.JSON(http.StatusOK, response)
}

// AutoScalingConfigUpdate save autoscaling config
func AutoScalingConfigUpdate(autoScalingRequest halib.AutoScalingConfigUpdateRequest, r render.Render) {
	var autoScalingResponse halib.AutoScalingConfigUpdateResponse

	err := autoscaling.SaveAutoScalingConfig(autoScalingRequest.Config, AutoScalingConfigFile)
	if err != nil {
		autoScalingResponse.Status = "NG"
	} else {
		autoScalingResponse.Status = "OK"
	}

	r.JSON(http.StatusOK, autoScalingResponse)
}

// AutoScalingInstanceRegister register autoscaling instance to dbms
func AutoScalingInstanceRegister(request halib.AutoScalingInstanceRegisterRequest, r render.Render) {
	log := util.HappoAgentLogger()
	var response halib.AutoScalingInstanceRegisterResponse

	if request.AutoScalingGroupName == "" || request.InstanceID == "" || request.IP == "" {
		response.Status = "error"
		response.Message = "missing parameter"
		log.Warnf("failed to register %s:%s", request.InstanceID, response.Message)
		r.JSON(http.StatusBadRequest, response)
		return
	}

	autoScalingList, err := autoscaling.GetAutoScalingConfig(AutoScalingConfigFile)
	if err != nil {
		response.Status = "error"
		response.Message = err.Error()
		log.Warnf("failed to register %s:%s", request.InstanceID, err.Error())
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	var autoScalingGroupName string
	var hostPrefix string
	for _, a := range autoScalingList.AutoScalings {
		if request.AutoScalingGroupName == a.AutoScalingGroupName {
			autoScalingGroupName = a.AutoScalingGroupName
			hostPrefix = a.HostPrefix
			break
		}
	}

	if autoScalingGroupName == "" {
		response.Status = "error"
		response.Message = "can't find autoscaling group name in config"
		log.Warnf("failed to register %s:%s", request.InstanceID, response.Message)
		r.JSON(http.StatusNotFound, response)
		return
	}

	alias, instanceData, err := autoscaling.RegisterAutoScalingInstance(autoScalingGroupName, hostPrefix, request.InstanceID, request.IP)
	if err != nil {
		response.Status = "error"
		response.Message = err.Error()
		log.Warnf("failed to register %s:%s", request.InstanceID, err.Error())
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	response.Status = "OK"
	response.Alias = alias
	response.InstanceData = instanceData

	log.Infof("register %s with alias %s", response.InstanceData.InstanceID, response.Alias)

	r.JSON(http.StatusOK, response)
}

// AutoScalingInstanceDeregister deregister autoscaling instance from dbms
func AutoScalingInstanceDeregister(request halib.AutoScalingInstanceDeregisterRequest, r render.Render) {
	log := util.HappoAgentLogger()
	var response halib.AutoScalingInstanceDeregisterResponse

	if request.InstanceID == "" {
		response.Status = "NG"
		response.Message = "instance_id required"
		r.JSON(http.StatusBadRequest, response)
		return
	}

	if err := autoscaling.DeregisterAutoScalingInstance(request.InstanceID); err != nil {
		response.Status = "NG"
		response.Message = err.Error()
		log.Warnf("failed to deregister %s:%s", request.InstanceID, err.Error())
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	response.Status = "OK"

	log.Infof("deregister %s", request.InstanceID)

	r.JSON(http.StatusOK, response)
}

// AutoScalingDelete delete autoscaling instances data
func AutoScalingDelete(request halib.AutoScalingDeleteRequest, r render.Render) {
	var response halib.AutoScalingDeleteResponse

	if request.AutoScalingGroupName == "" {
		response.Status = "error"
		response.Message = "autoscaling_gorup_name required"
		r.JSON(http.StatusBadRequest, response)
		return
	}

	autoScalingList, err := autoscaling.GetAutoScalingConfig(AutoScalingConfigFile)
	if err != nil {
		response.Status = "error"
		response.Message = err.Error()
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	var deleteAutoScalingGroup string
	for _, a := range autoScalingList.AutoScalings {
		if request.AutoScalingGroupName == a.AutoScalingGroupName {
			deleteAutoScalingGroup = a.AutoScalingGroupName
			break
		}
	}

	if deleteAutoScalingGroup == "" {
		response.Status = "error"
		response.Message = "can't find autoscaling group name in config"
		r.JSON(http.StatusNotFound, response)
		return
	}

	if err := autoscaling.DeleteAutoScaling(deleteAutoScalingGroup); err != nil {
		response.Status = "error"
		response.Message = err.Error()
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	response.Status = "OK"
	r.JSON(http.StatusOK, response)
}

// AutoScalingRefresh refresh autoscaling
func AutoScalingRefresh(request halib.AutoScalingRefreshRequest, r render.Render, client *autoscaling.AWSClient) {
	var response halib.AutoScalingRefreshResponse

	autoScalingList, err := autoscaling.GetAutoScalingConfig(AutoScalingConfigFile)
	if err != nil {
		response.Status = "error"
		response.Message = err.Error()
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	var refreshAutoScalingGroups = []struct {
		autoScalingGroupName string
		autoScalingCount     int
		hostPrefix           string
	}{}
	for _, a := range autoScalingList.AutoScalings {
		if request.AutoScalingGroupName == a.AutoScalingGroupName || request.AutoScalingGroupName == "" {
			refreshAutoScalingGroups = append(refreshAutoScalingGroups, struct {
				autoScalingGroupName string
				autoScalingCount     int
				hostPrefix           string
			}{
				autoScalingGroupName: a.AutoScalingGroupName,
				autoScalingCount:     a.AutoScalingCount,
				hostPrefix:           a.HostPrefix,
			})
		}
		if request.AutoScalingGroupName == a.AutoScalingGroupName {
			break
		}
	}
	if len(refreshAutoScalingGroups) == 0 {
		response.Status = "error"
		response.Message = "can't find autoscaling group name in config"
		r.JSON(http.StatusNotFound, response)
		return
	}

	var errors []string
	for _, a := range refreshAutoScalingGroups {
		err = autoscaling.RefreshAutoScalingInstances(client, a.autoScalingGroupName, a.hostPrefix, a.autoScalingCount)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to refresh for %s: %s", a.autoScalingGroupName, err.Error()))
		}
	}
	if len(errors) > 0 {
		response.Status = "error"
		response.Message = strings.Join(errors, ",")
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	response.Status = "OK"
	r.JSON(http.StatusOK, response)
}

// AutoScalingLeave deregister self node from autoscaling bastion.
// this handler is available only in agent running with autoscaling node.
func AutoScalingLeave(request halib.AutoScalingLeaveRequest, r render.Render, client *autoscaling.NodeAWSClient) {
	var response halib.AutoScalingLeaveResponse

	if err := autoscaling.LeaveAutoScalingGroup(client, AutoScalingBastionEndpoint); err != nil {
		response.Status = "error"
		response.Message = fmt.Sprintf("failed to leave: %s", err.Error())
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	response.Status = "OK"
	r.JSON(http.StatusOK, response)
}

// AutoScalingHealth checks auto scaling node is available
func AutoScalingHealth(req *http.Request, params martini.Params, r render.Render) {
	var response halib.AutoScalingHealthResponse
	alias := params["alias"]
	if alias == "" {
		response.Status = "error"
		response.Message = fmt.Sprintf("missing alias: %s", alias)
		r.JSON(http.StatusBadRequest, response)
		return
	}

	port := req.URL.Query().Get("port")
	if port == "" {
		port = fmt.Sprint(halib.DefaultAgentPort)
	}

	if _, err := strconv.Atoi(port); err != nil {
		response.Status = "error"
		response.Message = "port is must be specify integer"
		r.JSON(http.StatusBadRequest, response)
		return
	}

	ip, err := autoscaling.AliasToIP(alias)
	if err != nil {
		var errMsg string
		if err == leveldb.ErrNotFound {
			errMsg = fmt.Sprintf("alas not found: %s", alias)
		} else {
			errMsg = err.Error()
		}
		response.Status = "error"
		response.Message = errMsg
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	if ip == "" {
		response.Status = "OK"
		response.Message = fmt.Sprintf("%s has not been assigned instance", alias)
		r.JSON(http.StatusOK, response)
		return
	}

	endpoint := "https://" + ip + ":" + port
	res, err := util.RequestToCheckAvailableAPI(endpoint)
	if err != nil {
		response.Status = "error"
		response.Message = err.Error()
		response.IP = ip
		r.JSON(http.StatusInternalServerError, response)
		return
	}

	if res.StatusCode != http.StatusOK {
		response.Status = "error"
		response.Message = fmt.Sprint("node returns status HTTP ", res.StatusCode)
		response.IP = ip
		r.JSON(http.StatusOK, response)
		return
	}

	response.Status = "OK"
	response.Message = ""
	response.IP = ip

	r.JSON(http.StatusOK, response)
}
