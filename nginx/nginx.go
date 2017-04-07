package nginx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"text/template"

	"github.com/golang/glog"
)

// Configuration Type for NGINX Server
type Configuration uint8

const (
	// LocalCfg - running test on local machine (no NGINX server)
	LocalCfg = Configuration(iota)
	// StreamCfg - NGINX server configuration for TCP/UDP Streams only
	StreamCfg
	// HTTPCfg - NGINX server configuration for HTTP only
	HTTPCfg
	// StreamHTTPCfg - NGINX server configuration for TCP/UDP Streams and HTTP services
	StreamHTTPCfg
)

// NginxController Updates NGINX configuration, starts and reloads NGINX
type NginxController struct {
	nginxConfdPath string
	nginxCertsPath string
	cfgType        Configuration
	mainCfg        *NginxMainConfig
}

// NginxMainConfig describe the main NGINX configuration file
type NginxMainConfig struct {
	// Context: main directives
	Daemon         bool
	ErrorLogFile   string
	ErrorLogLevel  string
	Environment    map[string]string
	LockFile       string
	PidFile        string
	User           string
	Group          string
	WorkerPriority string
	// TODO: This needs to be a ConfigMap entry or CLI flag so that we can make
	//       it a function of the number of CPUs/vCPUs, and configure the POD
	//       resource limits propotionally for the scheduler.  For now this
	//       *should probably not* be set to 'auto'
	WorkerProcesses  string
	WorkingDirectory string

	EventContext NginxMainEventConfig

	DefaultStreamContext bool
	DefaultHTTPContext   bool
	HTTPContext          NginxMainHTTPConfig
}

// NginxMainEventConfig describe the main NGINX configuration file's 'events' context
type NginxMainEventConfig struct {
	// Context: events directives
	AcceptMutex       bool
	AcceptMutexDelay  string
	MultiAccept       bool
	WorkerConnections string
}

// NginxMainHTTPConfig describe the main NGINX configuration file's 'http' context
type NginxMainHTTPConfig struct {
	ServerNamesHashBucketSize string
	ServerNamesHashMaxSize    string
	LogFormat                 string
	HealthStatus              bool
	HTTPSnippets              []string
	// http://nginx.org/en/docs/http/ngx_http_ssl_module.html
	SSLProtocols           string
	SSLPreferServerCiphers bool
	SSLCiphers             string
	SSLDHParam             string
}

// NewNginxController creates a NGINX controller
func NewNginxController(cfgType Configuration, nginxConfPath string, healthStatus bool) (*NginxController, error) {
	ngxc := NginxController{
		nginxConfdPath: path.Join(nginxConfPath, "conf.d"),
		nginxCertsPath: path.Join(nginxConfPath, "ssl"),
		cfgType:        cfgType,
		mainCfg:        nil,
	}

	if cfgType != LocalCfg {
		cfg := &NginxMainConfig{
			Daemon:          true,
			ErrorLogFile:    "/var/log/nginx/error.log",
			ErrorLogLevel:   "warn",
			PidFile:         "/var/run/nginx.pid",
			User:            "nginx",
			Group:           "nginx",
			WorkerProcesses: "2",
			/* For future use potentially, can be scrubbed if prefered.
			Environment: map[string]string{
				"OPENSSL_ALLOW_PROXY_CERTS": "1",
			}, */
		}
		switch cfgType {
		case StreamCfg:
			cfg.DefaultStreamContext = true
			cfg.DefaultHTTPContext = false
		case HTTPCfg:
			createDir(ngxc.nginxCertsPath)
			cfg.DefaultStreamContext = false
			cfg.DefaultHTTPContext = true
			cfg.HTTPContext.ServerNamesHashMaxSize = NewDefaultHTTPContext().MainServerNamesHashMaxSize
			cfg.HTTPContext.HealthStatus = healthStatus
		}
		ngxc.mainCfg = cfg
		ngxc.UpdateMainConfigFile()
	}
	return &ngxc, nil
}

// Reload reloads NGINX
func (ngxc *NginxController) Reload() error {
	if ngxc.cfgType != LocalCfg {
		if err := shellOut("nginx -t"); err != nil {
			return fmt.Errorf("Reload: Invalid nginx configuration detected, not reloading: %s", err)
		}
		if err := shellOut("nginx -s reload"); err != nil {
			return fmt.Errorf("Reload: Reloading NGINX failed: %s", err)
		}
	} else {
		glog.V(3).Info("Reload: Reloading nginx")
	}
	return nil
}

// Start starts NGINX
func (ngxc *NginxController) Start() {
	if ngxc.cfgType != LocalCfg {
		if err := shellOut("nginx"); err != nil {
			glog.Fatalf("Failed to start nginx: %v", err)
		}
	} else {
		glog.V(3).Info("Starting nginx")
	}
}

func createDir(path string) {
	if err := os.Mkdir(path, os.ModeDir); err != nil {
		glog.Fatalf("Couldn't create directory %v: %v", path, err)
	}
}

func shellOut(cmd string) (err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	glog.V(3).Infof("executing %s", cmd)

	command := exec.Command("sh", "-c", cmd)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Start()
	if err != nil {
		return fmt.Errorf("Failed to execute %v, err: %v", cmd, err)
	}

	err = command.Wait()
	if err != nil {
		return fmt.Errorf("Command %v stdout: %q\nstderr: %q\nfinished with error: %v", cmd,
			stdout.String(), stderr.String(), err)
	}
	return nil
}

// UpdateMainConfigFile update the main NGINX configuration file
func (ngxc *NginxController) UpdateMainConfigFile() {
	tmpl, err := template.New("nginx.conf.tmpl").ParseFiles("nginx.conf.tmpl")
	if err != nil {
		glog.Fatalf("Failed to parse the main config template file: %v", err)
	}

	filename := "/etc/nginx/nginx.conf"
	if glog.V(2) {
		glog.Infof("Writing NGINX conf to %v", filename)
		tmpl.Execute(os.Stdout, ngxc.mainCfg)
	}

	if ngxc.cfgType != LocalCfg {
		w, err := os.Create(filename)
		if err != nil {
			glog.Fatalf("Failed to open %v: %v", filename, err)
		}
		defer w.Close()

		if err := tmpl.Execute(w, ngxc.mainCfg); err != nil {
			glog.Fatalf("Failed to write template %v", err)
		}
	}

	glog.V(3).Infof("The main NGINX configuration file had been updated")
}
