package command

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/client9/reopen"
	"github.com/codegangsta/cli"
	"github.com/codegangsta/martini-contrib/render"
	"github.com/codegangsta/martini-contrib/secure"
	"github.com/go-martini/martini"
	"github.com/heartbeatsjp/happo-agent/autoscaling"
	"github.com/heartbeatsjp/happo-agent/collect"
	"github.com/heartbeatsjp/happo-agent/db"
	"github.com/heartbeatsjp/happo-agent/halib"
	"github.com/heartbeatsjp/happo-agent/model"
	"github.com/heartbeatsjp/happo-agent/util"
	"github.com/martini-contrib/binding"
	"golang.org/x/net/netutil"
)

// --- Struct
type daemonListener struct {
	Timeout        int //second
	MaxConnections int
	Port           string
	Handler        http.Handler
	PublicKey      string
	PrivateKey     string
}

var autoScalingBastionEndpoint string
var autoScalingJoinWaitSeconds = halib.DefaultAutoScalingJoinWaitSeconds

// --- functions

// custom martini.Classic() for change change martini.Logger() to util.Logger()
func customClassic() *martini.ClassicMartini {
	/*
		- remove martini.Logging()
		- add happo_agent.martini_util.Logging()
	*/
	r := martini.NewRouter()
	m := martini.New()
	m.Use(util.MartiniCustomLogger())
	m.Use(martini.Recovery())
	m.Use(martini.Static("public"))
	m.MapTo(r, (*martini.Routes)(nil))
	m.Action(r.Handle)
	classic := new(martini.ClassicMartini)
	classic.Martini = m
	classic.Router = r
	return classic
}

// CmdDaemon implements subcommand `_daemon`
func CmdDaemon(c *cli.Context) {
	log := util.HappoAgentLogger()

	fp, err := reopen.NewFileWriter(c.String("logfile"))
	if err != nil {
		fmt.Println(err)
	}
	log.Info(fmt.Sprintf("switch log.Out to %s", c.String("logfile")))
	if !util.Production {
		log.Warn("MARTINI_ENV is not production. LogLevel force to debug")
		util.SetLogLevel(util.HappoAgentLogLevelDebug)
	}

	log.Out = fp
	sigHup := make(chan os.Signal, 1)
	signal.Notify(sigHup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-sigHup:
				fp.Reopen()
			}
		}
	}()

	m := customClassic()
	m.Use(render.Renderer())
	m.Use(util.ACL(c.StringSlice("allowed-hosts")))
	m.Use(
		secure.Secure(secure.Options{
			SSLRedirect:      true,
			DisableProdCheck: true,
		}))

	// AWS clients that behave for dummy when the daemon is not running within Amazon EC2.
	// When the daemon is running within AWS, overwrite with actual clients created by autoscaling.New*Client().
	// Finally, clients are injecting to the handler in both cases.
	var awsClient *autoscaling.AWSClient
	var nodeAWSClient *autoscaling.NodeAWSClient

	enableRequestStatusMiddlware := c.Bool("enable-requeststatus-middleware")
	if enableRequestStatusMiddlware {
		m.Use(util.MartiniRequestStatus())
	}

	// CPU Profiling
	if c.String("cpu-profile") != "" {
		cpuprofile := c.String("cpu-profile")
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
		cpuprof := make(chan os.Signal, 1)
		signal.Notify(cpuprof, os.Interrupt)
		go func() {
			for sig := range cpuprof {
				log.Printf("captured %v, stopping profiler and exiting...", sig)
				pprof.StopCPUProfile()
				os.Exit(1)
			}
		}()
	}

	dbfile := c.String("dbfile")
	db.Open(dbfile)
	defer db.Close()
	db.MetricsMaxLifetimeSeconds = c.Int64("metrics-max-lifetime-seconds")
	db.MachineStateMaxLifetimeSeconds = c.Int64("machine-state-max-lifetime-seconds")

	isAutoScalingNode := c.Bool("enable-autoscaling-node")
	if isAutoScalingNode {
		client, err := autoscaling.NewNodeAWSClient()
		if err == nil {
			nodeAWSClient = client
		} else if err == autoscaling.ErrNotRunningEC2 {
			log.Error("create aws client failed: ", err)
		} else {
			log.Fatal("create aws client failed: ", err)
		}
		m.Map(nodeAWSClient)

		path := c.String("autoscaling-parameter-store-path")
		if path != "" {
			p, err := client.GetAutoScalingNodeConfigParameters(path)
			if err != nil {
				log.Fatal(err.Error())
			}
			autoScalingBastionEndpoint = p.BastionEndpoint
			autoScalingJoinWaitSeconds = p.JoinWaitSeconds
		} else {
			autoScalingBastionEndpoint = c.String("autoscaling-bastion-endpoint")
			autoScalingJoinWaitSeconds = c.Int("autoscaling-join-wait-seconds")
		}

		if autoScalingBastionEndpoint == "" {
			log.Fatal(`missing "autoscaling-bastion-endpoint"`)
		}
		if autoScalingJoinWaitSeconds == 0 {
			log.Warn(`"autoscaling-join-wait-seconds is 0: please check your autoscaling node settings`)
		}
		log.Info(
			fmt.Sprintf(
				"running as autoscaling node (autoscaling-bastion-endpoint: %s, autoscaling-join-wait-seconds: %d)",
				autoScalingBastionEndpoint,
				autoScalingJoinWaitSeconds,
			),
		)

		model.AutoScalingBastionEndpoint = autoScalingBastionEndpoint

		go func() {
			time.Sleep(time.Duration(autoScalingJoinWaitSeconds) * time.Second)
			metricConfig, err := autoscaling.JoinAutoScalingGroup(client, autoScalingBastionEndpoint)
			if err != nil {
				log.Error(fmt.Sprintf("failed to join: %s", err.Error()))
				return
			}
			if err := collect.SaveMetricConfig(metricConfig, c.String("metric-config")); err != nil {
				log.Error(fmt.Sprintf("failed to save metric config: %s", err.Error()))
				return
			}
			log.Info(fmt.Sprintf("join succeed"))
		}()
	}

	model.SetProxyTimeout(c.Int64("proxy-timeout-seconds"))

	model.AppVersion = c.App.Version
	m.Get("/", func() string {
		return "OK"
	})

	util.CommandTimeout = time.Duration(c.Int("command-timeout"))
	model.MetricConfigFile = c.String("metric-config")
	model.AutoScalingConfigFile = c.String("autoscaling-config")
	if _, err := autoscaling.GetAutoScalingConfig(model.AutoScalingConfigFile); err == nil {
		client, err := autoscaling.NewAWSClient()
		if err == nil {
			awsClient = client
		} else if err == autoscaling.ErrNotRunningEC2 {
			log.Error("create aws client failed: ", err)
		} else {
			log.Fatal("create aws client failed: ", err)
		}
	}
	m.Map(awsClient)

	model.ErrorLogIntervalSeconds = c.Int64("error-log-interval-seconds")
	model.NagiosPluginPaths = c.String("nagios-plugin-paths")
	collect.SensuPluginPaths = c.String("sensu-plugin-paths")

	m.Post("/proxy", binding.Json(halib.ProxyRequest{}), model.Proxy)
	m.Post("/inventory", binding.Json(halib.InventoryRequest{}), model.Inventory)
	m.Post("/monitor", binding.Json(halib.MonitorRequest{}), model.Monitor)
	m.Post("/metric", binding.Json(halib.MetricRequest{}), model.Metric)
	m.Post("/metric/append", binding.Json(halib.MetricAppendRequest{}), model.MetricAppend)
	m.Post("/metric/config/update", binding.Json(halib.MetricConfigUpdateRequest{}), model.MetricConfigUpdate)
	if runtime.GOOS != "windows" {
		m.Post("/autoscaling/refresh", binding.Json(halib.AutoScalingRefreshRequest{}), model.AutoScalingRefresh)
		m.Post("/autoscaling/delete", binding.Json(halib.AutoScalingDeleteRequest{}), model.AutoScalingDelete)
		m.Post("/autoscaling/instance/register", binding.Json(halib.AutoScalingInstanceRegisterRequest{}), model.AutoScalingInstanceRegister)
		m.Post("/autoscaling/instance/deregister", binding.Json(halib.AutoScalingInstanceDeregisterRequest{}), model.AutoScalingInstanceDeregister)
		m.Post("/autoscaling/config/update", binding.Json(halib.AutoScalingConfigUpdateRequest{}), model.AutoScalingConfigUpdate)
		if isAutoScalingNode {
			m.Post("/autoscaling/leave", binding.Json(halib.AutoScalingLeaveRequest{}), model.AutoScalingLeave)
		}
		m.Get("/autoscaling", model.AutoScaling)
		m.Get("/autoscaling/resolve/:alias", model.AutoScalingResolve)
		m.Get("/autoscaling/health/:alias", model.AutoScalingHealth)
	}
	m.Get("/metric/status", model.MetricDataBufferStatus)
	m.Get("/status", model.Status)
	m.Get("/status/memory", model.MemoryStatus)
	if runtime.GOOS != "windows" {
		m.Get("/status/autoscaling", model.AutoScalingStatus)
		if enableRequestStatusMiddlware {
			m.Get("/status/request", model.RequestStatus)
		}
		m.Get("/machine-state", model.ListMachieState)
		m.Get("/machine-state/:key", model.GetMachineState)
	}

	// Listener
	var lis daemonListener
	lis.Port = fmt.Sprintf(":%d", c.Int("port"))
	lis.Handler = m
	lis.Timeout = halib.DefaultServerHTTPTimeout
	if lis.Timeout < int(c.Int64("proxy-timeout-seconds")) {
		lis.Timeout = int(c.Int64("proxy-timeout-seconds"))
	}
	if lis.Timeout < c.Int("command-timeout") {
		lis.Timeout = c.Int("command-timeout")
	}
	lis.MaxConnections = c.Int("max-connections")
	lis.PublicKey = c.String("public-key")
	lis.PrivateKey = c.String("private-key")
	go func() {
		err := lis.listenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	model.DisableCollectMetrics = c.Bool("disable-collect-metrics")
	util.HappoAgentLogger().Debug("disable-collect-metrics: ", model.DisableCollectMetrics)

	// Metric collect timer
	timeMetrics := time.NewTicker(time.Minute).C
	for {
		select {
		case <-timeMetrics:
			if !model.DisableCollectMetrics {
				err := collect.Metrics(c.String("metric-config"))
				if err != nil {
					log.Error(err)
				}
			}
		}
	}
}

// HTTPS Listener
func (l *daemonListener) listenAndServe() error {
	var err error

	cert := make([]tls.Certificate, 1)
	cert[0], err = tls.LoadX509KeyPair(l.PublicKey, l.PrivateKey)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			// tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
		NextProtos:               []string{"http/1.1"},
		Certificates:             cert,
	}

	listener, err := net.Listen("tcp", l.Port)
	if err != nil {
		return err
	}
	limitListener := netutil.LimitListener(listener, l.MaxConnections)
	tlsListener := tls.NewListener(limitListener, tlsConfig)

	httpConfig := &http.Server{
		TLSConfig:    tlsConfig,
		Addr:         l.Port,
		Handler:      l.Handler,
		ReadTimeout:  time.Duration(l.Timeout) * time.Second,
		WriteTimeout: time.Duration(l.Timeout) * time.Second,
	}

	return httpConfig.Serve(tlsListener)
}
