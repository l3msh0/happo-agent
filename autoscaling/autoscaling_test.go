package autoscaling

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/heartbeatsjp/happo-agent/db"
	"github.com/heartbeatsjp/happo-agent/halib"

	"github.com/heartbeatsjp/happo-agent/autoscaling/awsmock"
	"github.com/stretchr/testify/assert"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	leveldbUtil "github.com/syndtr/goleveldb/leveldb/util"
)

const TestConfigFile = "./testdata/autoscaling_test.yaml"
const TestFailConfigFile = "./testdata/autoscaling_test_fail.yaml"
const TestMultiConfigFile = "./testdata/autoscaling_test_multi.yaml"
const TestEmptyConfigFile = "./testdata/autoscaling_test_empty.yaml"
const TestMissingConfigFile = "./testdata/autoscaling_test_missing.yaml"

func teardown() {
	iter := db.DB.NewIterator(
		leveldbUtil.BytesPrefix(
			[]byte("ag-"),
		),
		nil,
	)
	for iter.Next() {
		key := iter.Key()
		db.DB.Delete(key, nil)
	}
	iter.Release()
}

func TestAutoScaling(t *testing.T) {
	var cases = []struct {
		name     string
		input    string
		expected []struct {
			name  string
			count int
		}
		isNormalTest bool
	}{
		{
			name:  "dummy-prod-ag",
			input: TestConfigFile,
			expected: []struct {
				name  string
				count int
			}{{"dummy-prod-ag", 10}},
			isNormalTest: true,
		},
		{
			name:  "dummy-prod-ag dummy-stg-ag",
			input: TestMultiConfigFile,
			expected: []struct {
				name  string
				count int
			}{{"dummy-prod-ag", 10}, {"dummy-stg-ag", 4}},
			isNormalTest: true,
		},
		{
			name:  "fail-dummy-prod-ag",
			input: TestFailConfigFile,
			expected: []struct {
				name  string
				count int
			}{{"fail-dummy-prod-ag", 10}},
			isNormalTest: true,
		},
		{
			name:  "dummy-empty-ag",
			input: TestEmptyConfigFile,
			expected: []struct {
				name  string
				count int
			}(nil),
			isNormalTest: true,
		},
		{
			name:  "dummy-missing-ag",
			input: TestMissingConfigFile,
			expected: []struct {
				name  string
				count int
			}(nil),
			isNormalTest: false,
		},
	}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}
	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)
	RefreshAutoScalingInstances(client, "fail-dummy-prod-ag", "fail-dummy-prod-app", 10)
	RefreshAutoScalingInstances(client, "dummy-stg-ag", "dummy-stg-app", 4)
	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			autoscaling, err := AutoScaling(c.input)
			var actual []struct {
				name  string
				count int
			}

			for _, a := range autoscaling {
				actual = append(actual, struct {
					name  string
					count int
				}{name: a.AutoScalingGroupName, count: len(a.Instances)})
			}

			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestSaveAutoScalingConfig(t *testing.T) {
	var cases = []struct {
		name         string
		input1       halib.AutoScalingConfig
		input2       string
		isNormalTest bool
	}{
		{
			name: "single autoscaling group",
			input1: halib.AutoScalingConfig{
				AutoScalings: []halib.AutoScalingConfigData{
					{
						AutoScalingGroupName: "dummy-prod-ag",
						AutoScalingCount:     10,
						HostPrefix:           "dummy-prod-app",
					},
				},
			},
			input2:       "./autoscaling_test_save.yaml",
			isNormalTest: true,
		},
	}

	for _, c := range cases {
		defer os.Remove(c.input2)
		t.Run(c.name, func(t *testing.T) {
			err := SaveAutoScalingConfig(c.input1, c.input2)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestGetAutoScalingConfig(t *testing.T) {
	var cases = []struct {
		name         string
		input        string
		expected     halib.AutoScalingConfig
		isNormalTest bool
	}{
		{
			name:  "single autoscalng group",
			input: TestConfigFile,
			expected: halib.AutoScalingConfig{
				AutoScalings: []halib.AutoScalingConfigData{
					{
						AutoScalingGroupName: "dummy-prod-ag",
						AutoScalingCount:     10,
						HostPrefix:           "dummy-prod-app",
					},
				},
			},
			isNormalTest: true,
		},
		{
			name:  "multi autoscaling group",
			input: TestMultiConfigFile,
			expected: halib.AutoScalingConfig{
				AutoScalings: []halib.AutoScalingConfigData{
					{
						AutoScalingGroupName: "dummy-prod-ag",
						AutoScalingCount:     10,
						HostPrefix:           "dummy-prod-app",
					},
					{
						AutoScalingGroupName: "dummy-stg-ag",
						AutoScalingCount:     4,
						HostPrefix:           "dummy-stg-app",
					},
				},
			},
			isNormalTest: true,
		},
		{
			name:  "empty config file",
			input: TestEmptyConfigFile,
			expected: halib.AutoScalingConfig{
				AutoScalings: []halib.AutoScalingConfigData(nil),
			},
			isNormalTest: true,
		},
		{
			name:  "missing config file",
			input: TestMissingConfigFile,
			expected: halib.AutoScalingConfig{
				AutoScalings: []halib.AutoScalingConfigData(nil),
			},
			isNormalTest: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := GetAutoScalingConfig(c.input)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestRegisterAutoScalingInstance(t *testing.T) {
	var cases = []struct {
		name     string
		input1   string
		input2   string
		input3   string
		input4   string
		expected struct {
			alias        string
			instanceData halib.InstanceData
		}
		isNormalTest bool
	}{
		{
			name:   "dummy-prod-ag",
			input1: "dummy-prod-ag",
			input2: "dummy-prod-app",
			input3: "i-zzzzzz",
			input4: "192.0.2.99",
			expected: struct {
				alias        string
				instanceData halib.InstanceData
			}{
				alias: "dummy-prod-ag-dummy-prod-app-11",
				instanceData: halib.InstanceData{
					InstanceID:   "i-zzzzzz",
					IP:           "192.0.2.99",
					MetricConfig: halib.MetricConfig{},
				},
			},
			isNormalTest: true,
		},
		{
			name:   "dummy-prod-ag already instance",
			input1: "dummy-prod-ag",
			input2: "dummy-prod-app",
			input3: "i-aaaaaa",
			input4: "192.0.2.11",
			expected: struct {
				alias        string
				instanceData halib.InstanceData
			}{
				alias: "",
				instanceData: halib.InstanceData{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
			isNormalTest: false,
		},
		{
			name:   "dummy-stg-ag no empty alias",
			input1: "dummy-stg-ag",
			input2: "dummy-stg-app",
			input3: "i-zzzzzz",
			input4: "192.0.2.99",
			expected: struct {
				alias        string
				instanceData halib.InstanceData
			}{
				alias: "",
				instanceData: halib.InstanceData{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
			isNormalTest: false,
		},
		{
			name:   "dummy-missing-ag",
			input1: "dummy-missing-ag",
			input2: "dummy-missing-app",
			input3: "i-zzzzzz",
			input4: "192.0.2.99",
			expected: struct {
				alias        string
				instanceData halib.InstanceData
			}{
				alias: "",
				instanceData: halib.InstanceData{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
			isNormalTest: false,
		},
	}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}
	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 20)
	RefreshAutoScalingInstances(client, "dummy-stg-ag", "dummy-stg-app", 4)
	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actualAlias, actualInstanceData, err := RegisterAutoScalingInstance(c.input1, c.input2, c.input3, c.input4)
			assert.Equal(t, c.expected.alias, actualAlias)
			assert.Equal(t, c.expected.instanceData, actualInstanceData)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestDeregisterAutoScalingInstance(t *testing.T) {
	var cases = []struct {
		name         string
		input        string
		isNormalTest bool
	}{
		{
			name:         "deregister i-aaaaaa",
			input:        "i-aaaaaa",
			isNormalTest: true,
		},
		{
			name:         "deregister i-zzzzzz",
			input:        "i-zzzzzz",
			isNormalTest: false,
		},
	}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}
	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)
	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := DeregisterAutoScalingInstance(c.input)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}

}

func TestRefreshAutoScalingInstances(t *testing.T) {
	var cases = []struct {
		name     string
		input1   string
		input2   string
		input3   int
		expected []halib.InstanceData
	}{
		{
			name:   "dummy-prod-ag",
			input1: "dummy-prod-ag",
			input2: "dummy-prod-app",
			input3: 10,
			expected: []halib.InstanceData{
				{
					InstanceID:   "i-aaaaaa",
					IP:           "192.0.2.11",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-bbbbbb",
					IP:           "192.0.2.12",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-cccccc",
					IP:           "192.0.2.13",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-dddddd",
					IP:           "192.0.2.14",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-eeeeee",
					IP:           "192.0.2.15",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-ffffff",
					IP:           "192.0.2.16",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-gggggg",
					IP:           "192.0.2.17",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-hhhhhh",
					IP:           "192.0.2.18",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-iiiiii",
					IP:           "192.0.2.19",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-jjjjjj",
					IP:           "192.0.2.20",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
		{
			name:   "fail-dummy-prod-ag",
			input1: "fail-dummy-prod-ag",
			input2: "fail-dummy-prod-app",
			input3: 10,
			expected: []halib.InstanceData{
				{
					InstanceID:   "i-aaaaaa",
					IP:           "192.0.2.11",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-cccccc",
					IP:           "192.0.2.13",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-eeeeee",
					IP:           "192.0.2.15",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-ffffff",
					IP:           "192.0.2.16",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-gggggg",
					IP:           "192.0.2.17",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-hhhhhh",
					IP:           "192.0.2.18",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-jjjjjj",
					IP:           "192.0.2.20",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
		{
			name:   "dummy-stg-ag",
			input1: "dummy-stg-ag",
			input2: "dummy-stg-app",
			input3: 4,
			expected: []halib.InstanceData{
				{
					InstanceID:   "i-kkkkkk",
					IP:           "192.0.2.21",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-llllll",
					IP:           "192.0.2.22",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-mmmmmm",
					IP:           "192.0.2.23",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "i-nnnnnn",
					IP:           "192.0.2.24",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
		{
			name:   "allfail-dummy-stg-ag",
			input1: "allfail-dummy-stg-ag",
			input2: "allfail-dummy-stg-app",
			input3: 4,
			expected: []halib.InstanceData{
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
		{
			name:   "nill-dummy-stg-ag",
			input1: "nill-dummy-stg-ag",
			input2: "nill-dummy-stg-app",
			input3: 4,
			expected: []halib.InstanceData{
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
		{
			name:   "missing instance",
			input1: "dummy-missing-ag",
			input2: "dummy-missing-app",
			input3: 4,
			expected: []halib.InstanceData{
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
				{
					InstanceID:   "",
					IP:           "",
					MetricConfig: halib.MetricConfig{},
				},
			},
		},
	}

	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &AWSClient{
				SvcEC2:         &awsmock.MockEC2Client{},
				SvcAutoscaling: &awsmock.MockAutoScalingClient{},
			}
			err := RefreshAutoScalingInstances(client, c.input1, c.input2, c.input3)
			assert.Nil(t, err)

			iter := db.DB.NewIterator(
				leveldbUtil.BytesPrefix(
					[]byte(fmt.Sprintf("ag-%s-%s-", c.input1, c.input2)),
				),
				nil,
			)
			var actual []halib.InstanceData
			for iter.Next() {
				value := iter.Value()

				var instanceData halib.InstanceData
				dec := gob.NewDecoder(bytes.NewReader(value))
				dec.Decode(&instanceData)
				actual = append(actual, instanceData)
			}
			iter.Release()

			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestDeleteAutoScaling(t *testing.T) {
	var cases = []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "dummy-prod-ag",
			input:    "dummy-prod-ag",
			expected: 0,
		},
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
	}
	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}
	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)
	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := DeleteAutoScaling(c.input)
			assert.Nil(t, err)

			iter := db.DB.NewIterator(
				leveldbUtil.BytesPrefix(
					[]byte(fmt.Sprintf("ag-%s", c.input)),
				),
				nil,
			)
			var actual []string
			for iter.Next() {
				key := iter.Key()
				actual = append(actual, string(key))
			}
			iter.Release()

			assert.Equal(t, c.expected, len(actual))

		})
	}
}

func TestAliasToIP(t *testing.T) {
	var cases = []struct {
		name         string
		input        string
		expected     string
		isNormalTest bool
	}{
		{
			name:         "dummy-prod-ag-dummy-prod-app-01",
			input:        "dummy-prod-ag-dummy-prod-app-01",
			expected:     "192.0.2.11",
			isNormalTest: true,
		},
		{
			name:         "dummy-prod-ag-dummy-prod-app-99",
			input:        "dummy-prod-ag-dummy-prod-app-99",
			expected:     "",
			isNormalTest: false,
		},
	}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}
	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)
	defer teardown()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := AliasToIP(c.input)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestJoinAutoScalingGroup1(t *testing.T) {
	statusOKResponse := `
{
  "status": "ok",
  "message": "",
  "alias": "dummy-prod-ag-1",
  "instance_data": {
    "instance_id": "i-aaaaaa",
    "ip": "192.0.2.11",
    "metric_config": {
      "metrics": [
        {
          "hostname": "dummy-prod-ag-1",
          "plugins": [
            {
              "plugin_name": "metrics_test_plugin",
			  "plugin_option": ""
            }
          ]
        }
      ]
    }
  }
}
`

	statusErrorResponse := `
{
  "status": "error",
  "message": "dummy error",
  "alias": "",
  "instance_data": {
    "instance_id": "",
    "ip": "",
    "metric_config": {
      "metrics": [
        {
          "hostname": "",
          "plugins": [
            {
              "plugin_name": "",
			  "plugin_option": ""
            }
          ]
        }
      ]
    }
  }s
}
`

	var cases = []struct {
		name                   string
		ec2MetaDataisAvailable bool
		ec2MetaDatahasError    bool
		statusCode             int
		dummyResponse          string
		expected               halib.MetricConfig
		isNormalTest           bool
	}{
		{
			name:                   "default",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    false,
			statusCode:             http.StatusOK,
			dummyResponse:          statusOKResponse,
			expected: halib.MetricConfig{
				Metrics: []struct {
					Hostname string `yaml:"hostname" json:"Hostname"`
					Plugins  []struct {
						PluginName   string `yaml:"plugin_name" json:"Plugin_Name"`
						PluginOption string `yaml:"plugin_option" json:"Plugin_Option"`
					} `yaml:"plugins" json:"Plugins"`
				}{
					{
						Hostname: "dummy-prod-ag-1",
						Plugins: []struct {
							PluginName   string `yaml:"plugin_name" json:"Plugin_Name"`
							PluginOption string `yaml:"plugin_option" json:"Plugin_Option"`
						}{
							{
								PluginName:   "metrics_test_plugin",
								PluginOption: "",
							},
						},
					},
				},
			},
			isNormalTest: true,
		},
		{
			name:                   "error response",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    false,
			statusCode:             http.StatusInternalServerError,
			dummyResponse:          statusErrorResponse,
			expected:               halib.MetricConfig{},
			isNormalTest:           false,
		},
		{
			name:                   "ec2metadata is not available",
			ec2MetaDataisAvailable: false,
			ec2MetaDatahasError:    false,
			dummyResponse:          statusErrorResponse,
			expected:               halib.MetricConfig{},
			isNormalTest:           false,
		},
		{
			name:                   "ec2metadata has error",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    true,
			dummyResponse:          statusErrorResponse,
			expected:               halib.MetricConfig{},
			isNormalTest:           false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(c.statusCode)
						fmt.Fprint(w, statusOKResponse)
					}))

			re, _ := regexp.Compile("([a-z]+)://([A-Za-z0-9.]+):([0-9]+)(.*)")
			found := re.FindStringSubmatch(ts.URL)
			host := found[2]
			port, _ := strconv.Atoi(found[3])
			endpoint := fmt.Sprintf("https://%s:%d", host, port)

			client := &NodeAWSClient{
				SvcEC2Metadata: &awsmock.MockEC2MetadataClient{
					IsAvailable: c.ec2MetaDataisAvailable,
					HasError:    c.ec2MetaDatahasError,
				},
				SvcAutoScaling: &awsmock.MockAutoScalingClient{},
			}

			actual, err := JoinAutoScalingGroup(client, endpoint)

			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestLeaveAutoScalingGroup(t *testing.T) {
	statusOKResponse := `
{
  "status": "OK",
  "message": "",
}
`

	statusErrorResponse := `
{
  "status": "NG",
  "message": "dummy error",
`

	var cases = []struct {
		name                   string
		ec2MetaDataisAvailable bool
		ec2MetaDatahasError    bool
		statusCode             int
		dummyResponse          string
		isNormalTest           bool
	}{
		{
			name:                   "default",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    false,
			statusCode:             http.StatusOK,
			dummyResponse:          statusOKResponse,
			isNormalTest:           true,
		},
		{
			name:                   "error response",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    false,
			statusCode:             http.StatusInternalServerError,
			dummyResponse:          statusErrorResponse,
			isNormalTest:           false,
		},
		{
			name:                   "ec2metadata is not available",
			ec2MetaDataisAvailable: false,
			ec2MetaDatahasError:    false,
			dummyResponse:          statusErrorResponse,
			isNormalTest:           false,
		},
		{
			name:                   "ec2metadata has error",
			ec2MetaDataisAvailable: true,
			ec2MetaDatahasError:    true,
			dummyResponse:          statusErrorResponse,
			isNormalTest:           false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewTLSServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(c.statusCode)
						fmt.Fprint(w, statusOKResponse)
					}))

			re, _ := regexp.Compile("([a-z]+)://([A-Za-z0-9.]+):([0-9]+)(.*)")
			found := re.FindStringSubmatch(ts.URL)
			host := found[2]
			port, _ := strconv.Atoi(found[3])
			endpoint := fmt.Sprintf("https://%s:%d", host, port)

			client := &NodeAWSClient{
				SvcEC2Metadata: &awsmock.MockEC2MetadataClient{
					IsAvailable: c.ec2MetaDataisAvailable,
					HasError:    c.ec2MetaDatahasError,
				},
				SvcAutoScaling: &awsmock.MockAutoScalingClient{},
			}

			err := LeaveAutoScalingGroup(client, endpoint)

			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestCompareInstances(t *testing.T) {
	var cases = []struct {
		name         string
		input        []string
		expected     []string
		isNormalTest bool
	}{
		{
			name:         "dummy-prod-ag",
			input:        []string{"dummy-prod-ag", "dummy-prod-app"},
			expected:     []string(nil),
			isNormalTest: true,
		},
		{
			name:         "dummy-empty-ag",
			input:        []string{"dummy-empty-ag", "dummy-empty-app"},
			expected:     []string(nil),
			isNormalTest: true,
		},
		{
			name:         "dummy-missing-ag",
			input:        []string{"dummy-missing-ag", "dummy-missing-app"},
			expected:     []string(nil),
			isNormalTest: true,
		},
		{
			name:         "empty input",
			input:        []string{"", ""},
			expected:     []string{},
			isNormalTest: false,
		},
	}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}

	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diff, err := CompareInstances(client, c.input[0], c.input[1])
			assert.Equal(t, c.expected, diff)
			if c.isNormalTest {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}

func TestCompareInstances_Diff(t *testing.T) {
	input := []string{"dummy-prod-ag", "dummy-prod-app"}
	expected := []string{"i-aaaaaa"}

	client := &AWSClient{
		SvcEC2:         &awsmock.MockEC2Client{},
		SvcAutoscaling: &awsmock.MockAutoScalingClient{},
	}

	RefreshAutoScalingInstances(client, "dummy-prod-ag", "dummy-prod-app", 10)
	iter := db.DB.NewIterator(
		leveldbUtil.BytesPrefix(
			[]byte("ag-"),
		),
		nil,
	)
	for iter.Next() {
		var data halib.InstanceData
		v := iter.Value()
		dec := gob.NewDecoder(bytes.NewReader(v))
		dec.Decode(&data)

		if data.InstanceID == "i-aaaaaa" {
			db.DB.Delete(iter.Key(), nil)
		}
	}
	iter.Release()

	diff, err := CompareInstances(client, input[0], input[1])
	assert.Equal(t, expected, diff)
	assert.Nil(t, err)
}

func TestMain(m *testing.M) {
	//Mock
	DB, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		os.Exit(1)
	}
	db.DB = DB
	os.Exit(m.Run())

	db.DB.Close()
}
