package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/yaml.v3"

	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/K-Yo/splunk_exporter/config"
	"github.com/K-Yo/splunk_exporter/exporter"
)

var (
	sc = config.NewSafeConfig(prometheus.DefaultRegisterer)

	configFile     = kingpin.Flag("config.file", "Blackbox exporter configuration file.").Default("splunk_exporter.yml").String()
	timeoutOffset  = kingpin.Flag("timeout-offset", "Offset to subtract from timeout in seconds.").Default("0.5").Float64()
	configCheck    = kingpin.Flag("config.check", "If true validate the config file and then exit.").Default().Bool()
	logLevelProber = kingpin.Flag("log.prober", "Log level from probe requests. One of: [debug, info, warn, error, none]").Default("none").String()
	historyLimit   = kingpin.Flag("history.limit", "The maximum amount of items to keep in the history.").Default("100").Uint()
	externalURL    = kingpin.Flag("web.external-url", "The URL under which Blackbox exporter is externally reachable (for example, if Blackbox exporter is served via a reverse proxy). Used for generating relative and absolute links back to Blackbox exporter itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Blackbox exporter. If omitted, relevant URL components will be derived automatically.").PlaceHolder("<url>").String()
	routePrefix    = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").PlaceHolder("<path>").String()
	toolkitFlags   = webflag.AddFlags(kingpin.CommandLine, ":9115")
)

func init() {
	prometheus.MustRegister(
		versioncollector.NewCollector("splunk_exporter"),
	)
}

func main() {
	os.Exit(run())
}

func run() int {

	kingpin.CommandLine.UsageWriter(os.Stdout)
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("splunk_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting splunk_exporter", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())

	if err := sc.ReloadConfig(*configFile, logger); err != nil {
		level.Error(logger).Log("msg", "Error loading config", "err", err)
		return 1
	}

	// register exporter
	opts := exporter.SplunkOpts{
		URI:      sc.C.URL,
		Token:    sc.C.Token,
		Insecure: sc.C.Insecure,
	}
	exp, err := exporter.New(opts, logger, sc.C.Metrics)
	if err != nil {
		level.Error(logger).Log("msg", "could not create exporter", "err", err)
		return 1
	}

	prometheus.MustRegister(exp)

	// Infer or set Splunk exporter externalURL
	listenAddrs := toolkitFlags.WebListenAddresses
	if *externalURL == "" && *toolkitFlags.WebSystemdSocket {
		level.Error(logger).Log("msg", "Cannot automatically infer external URL with systemd socket listener. Please provide --web.external-url")
		return 1
	} else if *externalURL == "" && len(*listenAddrs) > 1 {
		level.Info(logger).Log("msg", "Inferring external URL from first provided listen address")
	}
	beURL, err := computeExternalURL(*externalURL, (*listenAddrs)[0])
	if err != nil {
		level.Error(logger).Log("msg", "failed to determine external URL", "err", err)
		return 1
	}
	level.Debug(logger).Log("externalURL", beURL.String())

	// Default -web.route-prefix to path of -web.external-url.
	if *routePrefix == "" {
		*routePrefix = beURL.Path
	}

	// routePrefix must always be at least '/'.
	*routePrefix = "/" + strings.Trim(*routePrefix, "/")
	// routePrefix requires path to have trailing "/" in order
	// for browsers to interpret the path-relative path correctly, instead of stripping it.
	if *routePrefix != "/" {
		*routePrefix = *routePrefix + "/"
	}
	level.Debug(logger).Log("routePrefix", *routePrefix)

	hup := make(chan os.Signal, 1)
	reloadCh := make(chan chan error)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-hup:
				if err := sc.ReloadConfig(*configFile, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					continue
				}
				level.Info(logger).Log("msg", "Reloaded config file")
			case rc := <-reloadCh:
				if err := sc.ReloadConfig(*configFile, logger); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					rc <- err
				} else {
					level.Info(logger).Log("msg", "Reloaded config file")
					rc <- nil
				}
			}
			exp.UpdateConf(sc.C)
		}
	}()

	// Match Prometheus behavior and redirect over externalURL for root path only
	// if routePrefix is different than "/"
	if *routePrefix != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, beURL.String(), http.StatusFound)
		})
	}

	http.HandleFunc(path.Join(*routePrefix, "/-/reload"),
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				fmt.Fprintf(w, "This endpoint requires a POST request.\n")
				return
			}

			rc := make(chan error)
			reloadCh <- rc
			if err := <-rc; err != nil {
				http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
			}
		})
	http.Handle(path.Join(*routePrefix, "/metrics"), promhttp.Handler())
	http.HandleFunc(path.Join(*routePrefix, "/-/healthy"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Healthy"))
	})
	http.HandleFunc(*routePrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>
    <head><title>Blackbox Exporter</title></head>
    <body>
    <h1>Splunk Exporter</h1>
    <p><a href="metrics">Metrics</a></p>
    <p><a href="config">Configuration</a></p>`))
	})

	http.HandleFunc(path.Join(*routePrefix, "/logs"), func(w http.ResponseWriter, r *http.Request) {
		// id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		// if err != nil {
		// 	id = -1
		// }
		// target := r.URL.Query().Get("target")
		// if err == nil && target != "" {
		// 	http.Error(w, "Probe id and target can't be defined at the same time", http.StatusBadRequest)
		// 	return
		// }
		// if id == -1 && target == "" {
		// 	http.Error(w, "Probe id or target must be defined as http query parameters", http.StatusBadRequest)
		// 	return
		// }
		// result := new(prober.Result)
		// if target != "" {
		// 	result = rh.GetByTarget(target)
		// 	if result == nil {
		// 		http.Error(w, "Probe target not found", http.StatusNotFound)
		// 		return
		// 	}
		// } else {
		// 	result = rh.GetById(id)
		// 	if result == nil {
		// 		http.Error(w, "Probe id not found", http.StatusNotFound)
		// 		return
		// 	}
		// }

		w.Header().Set("Content-Type", "text/plain")
		// w.Write([]byte(result.DebugOutput))
	})

	http.HandleFunc(path.Join(*routePrefix, "/config"), func(w http.ResponseWriter, r *http.Request) {
		sc.RLock()
		c, err := yaml.Marshal(sc.C)
		sc.RUnlock()
		if err != nil {
			level.Warn(logger).Log("msg", "Error marshalling configuration", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write(c)
	})
	srv := &http.Server{}
	srvc := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}

}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
}

// computeExternalURL computes a sanitized external URL from a raw input. It infers unset
// URL parts from the OS and the given listen address.
func computeExternalURL(u, listenAddr string) (*url.URL, error) {
	if u == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return nil, err
		}
		u = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if startsOrEndsWithQuote(u) {
		return nil, errors.New("URL must not begin or end with quotes")
	}

	eu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(eu.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	eu.Path = ppref

	return eu, nil
}
