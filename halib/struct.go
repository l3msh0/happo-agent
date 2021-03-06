package halib

// --- Struct

// MetricsData is actual metrics
type MetricsData struct {
	HostName  string             `json:"hostname"`
	Timestamp int64              `json:"timestamp"`
	Metrics   map[string]float64 `json:"metrics"`
}

// InventoryData is actual inventory
type InventoryData struct {
	GroupName     string `json:"group_name"`
	IP            string `json:"ip"`
	Command       string `json:"command"`
	CommandOption string `json:"command_option"`
	ReturnCode    int    `json:"return_code"`
	ReturnValue   string `json:"return_value"`
	Created       string `json:"created"`
}

// InstanceData is actual instance
type InstanceData struct {
	IP           string       `json:"ip"`
	InstanceID   string       `json:"instance_id"`
	MetricConfig MetricConfig `json:"metric_config"`
}

// AutoScalingData name and actual instance data
type AutoScalingData struct {
	AutoScalingGroupName string `json:"autoscaling_group_name"`
	Instances            []struct {
		Alias        string       `json:"alias"`
		InstanceData InstanceData `json:"instance_data"`
	} `json:"instances"`
}

// AutoScalingConfigData is actual autoscaling config data
type AutoScalingConfigData struct {
	AutoScalingGroupName string `yaml:"autoscaling_group_name" json:"autoscaling_group_name"`
	AutoScalingCount     int    `yaml:"autoscaling_count" json:"autoscaling_count"`
	HostPrefix           string `yaml:"host_prefix" json:"host_prefix"`
}

// AutoScalingNodeConfigParameters is struct of parameters for autoscaling node
type AutoScalingNodeConfigParameters struct {
	BastionEndpoint string
	JoinWaitSeconds int
}

// AutoScalingStatus represents the status of autoscaling group
type AutoScalingStatus struct {
	AutoScalingGroupName string `json:"autoscaling_group_name"`
	Status               string `json:"status"`
	Message              string `json:"message"`
}

// --- Request Parameter

// ProxyRequest is /proxy API
type ProxyRequest struct {
	ProxyHostPort []string `json:"proxy_hostport"`
	RequestType   string   `json:"request_type"`
	RequestJSON   []byte   `json:"request_json"`
}

// MonitorRequest is /monitor API
type MonitorRequest struct {
	APIKey       string `json:"apikey"`
	PluginName   string `json:"plugin_name"  binding:"required"`
	PluginOption string `json:"plugin_option"`
}

// MetricRequest is /metric API
type MetricRequest struct {
	APIKey string `json:"apikey"`
}

// MetricAppendRequest is /metric/append API
type MetricAppendRequest struct {
	APIKey     string        `json:"apikey"`
	MetricData []MetricsData `json:"metric_data"`
}

// MetricConfigUpdateRequest is /metric/config/update API
type MetricConfigUpdateRequest struct {
	APIKey string       `json:"apikey"`
	Config MetricConfig `json:"config"`
}

// InventoryRequest is /inventory API
type InventoryRequest struct {
	APIKey        string `json:"apikey"`
	Command       string `json:"command"`
	CommandOption string `json:"command_option"`
}

// ManageRequest is Manage API
type ManageRequest struct {
	APIKey   string           `json:"apikey"`
	Hostdata CrawlConfigAgent `json:"hostdata"`
}

// AutoScalingRefreshRequest is /autoscaling/refresh API
type AutoScalingRefreshRequest struct {
	APIKey               string `json:"apikey"`
	AutoScalingGroupName string `json:"autoscaling_group_name"`
}

// AutoScalingDeleteRequest is /autoscaling/delete API
type AutoScalingDeleteRequest struct {
	APIKey               string `json:"apikey"`
	AutoScalingGroupName string `json:"autoscaling_group_name"`
}

// AutoScalingInstanceRegisterRequest is /autoscaling/instance/register API
type AutoScalingInstanceRegisterRequest struct {
	APIKey               string `json:"apikey"`
	InstanceID           string `json:"instance_id"`
	IP                   string `json:"ip"`
	AutoScalingGroupName string `json:"autoscaling_group_name"`
}

// AutoScalingInstanceDeregisterRequest is /autoscaling/instance/delete API
type AutoScalingInstanceDeregisterRequest struct {
	APIKey     string `json:"apikey"`
	InstanceID string `json:"instance_id"`
}

// AutoScalingConfigUpdateRequest is /autoscaling/config/update API
type AutoScalingConfigUpdateRequest struct {
	APIKey string            `json:"apikey"`
	Config AutoScalingConfig `json:"config"`
}

// AutoScalingLeaveRequest is /autoscaling/leave API
type AutoScalingLeaveRequest struct {
	APIKey string `json:"apikey"`
}

// --- Response Parameter

// MonitorResponse is /monitor API
type MonitorResponse struct {
	ReturnValue int    `json:"return_value"`
	Message     string `json:"message"`
}

// MetricResponse is /metric API
type MetricResponse struct {
	MetricData []MetricsData `json:"metric_data"`
	Message    string        `json:"message"`
}

// MetricAppendResponse is /metric/append API
type MetricAppendResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// MetricConfigUpdateResponse is /metric/config/update API
type MetricConfigUpdateResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// InventoryResponse is /inventory API
type InventoryResponse struct {
	ReturnCode  int    `json:"return_code"`
	ReturnValue string `json:"return_value"`
}

// ManageResponse is Manage API
type ManageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingResponse is /autoscaling API
type AutoScalingResponse struct {
	AutoScaling []AutoScalingData `json:"autoscaling"`
}

// AutoScalingResolveResponse is /autoscaling/resolve/:alias API
type AutoScalingResolveResponse struct {
	Status string `jspn:"status"`
	IP     string `json:"ip"`
}

// AutoScalingRefreshResponse is /autoscaling/refresh API
type AutoScalingRefreshResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingDeleteResponse is /autoscaling/delete API
type AutoScalingDeleteResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingInstanceRegisterResponse is /autoscaling/instance/register API
type AutoScalingInstanceRegisterResponse struct {
	Status       string       `json:"status"`
	Message      string       `json:"message"`
	Alias        string       `json:"alias"`
	InstanceData InstanceData `json:"instance_data"`
}

// AutoScalingInstanceDeregisterResponse is /autoscaling/instance/delete API
type AutoScalingInstanceDeregisterResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingConfigUpdateResponse is /autoscaling/config/update API
type AutoScalingConfigUpdateResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingLeaveResponse is /autoscaling/leave API
type AutoScalingLeaveResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AutoScalingHealthResponse is /autoscaling/health/:alias API
type AutoScalingHealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	IP      string `json:"ip"`
}

// StatusResponse is /status API
type StatusResponse struct {
	AppVersion            string            `json:"app_version"`
	UptimeSeconds         int64             `json:"uptime_seconds"`
	DisableCollectMetrics bool              `json:"disable_collect_metrics"`
	NumGoroutine          int               `json:"num_goroutine"`
	MetricBufferStatus    map[string]int64  `json:"metric_buffer_status"`
	Callers               []string          `json:"callers"`
	LevelDBProperties     map[string]string `json:"leveldb_properties"`
}

// RequestStatusResponse is /status/request API
type RequestStatusResponse struct {
	Last1 []RequestStatusData `json:"last1"`
	Last5 []RequestStatusData `json:"last5"`
}

// RequestStatusData is data part of RequestStatusResponse
type RequestStatusData struct {
	URL    string         `json:"url"`
	Counts map[int]uint64 `json:"counts"`
}
