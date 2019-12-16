package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	core_v1 "k8s.io/api/core/v1"

	"github.com/wish/pod-netstat-exporter/pkg/docker"
	"github.com/wish/pod-netstat-exporter/pkg/kubelet"
	"github.com/wish/pod-netstat-exporter/pkg/metrics"
	"github.com/wish/pod-netstat-exporter/pkg/netstat"
)

type ops struct {
	kubelet.ClientConfig
	LogLevel      string  `long:"log-level" env:"LOG_LEVEL" description:"Log level" default:"info"`
	RateLimit     float64 `long:"rate-limit" env:"RATE_LIMIT" description:"The number of /metrics requests served per second" default:"3"`
	BindAddr      string  `long:"bind-address" short:"p" env:"BIND_ADDRESS" default:":9657" description:"address for binding metrics listener"`
	HostMountPath string  `long:"host-mount-path" env:"HOST_MOUNT_PATH" default:"/host" description:"The path where the host filesystem is mounted"`
}

func setupLogging(logLevel string) {
	// Use log level
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.Fatalf("Unknown log level %s: %v", logLevel, err)
	}
	logrus.SetLevel(level)

	// Set the log format to have a reasonable timestamp
	formatter := &logrus.TextFormatter{
		FullTimestamp: true,
	}
	logrus.SetFormatter(formatter)
}

// All containers in a pod share the same netns, so get the PID and then the statistics
// from the first pod
func getPodNetstats(opts *ops, pod *core_v1.Pod) (*netstat.NetStats, error) {
	logrus.Tracef("Getting stats for pod %v", pod.Name)
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil, fmt.Errorf("No containers in pod")
	}
	container := pod.Status.ContainerStatuses[0].ContainerID
	pid, err := docker.ContainerToPID(opts.HostMountPath, container)
	if err != nil {
		return nil, fmt.Errorf("Error getting pid for container %v: %v", container, err)
	}
	logrus.Tracef("Container %v of pod %v has PID %v", container, pod.Name, pid)
	stats, err := netstat.GetStats(opts.HostMountPath, pid)
	return &stats, err
}

func allPodStats(opts *ops, client *kubelet.Client) ([]*metrics.PodStats, error) {
	podStats := []*metrics.PodStats{}

	p, err := client.GetPodList()
	if err != nil {
		return podStats, fmt.Errorf("Error getting pod list: %v", err)
	}

	// Actually fetch the per-pod statistics
	for _, pod := range p.Items {
		if pod.Spec.HostNetwork {
			logrus.Tracef("Pod %v has hostNetwork: true, cannot fetch per-pod network metrics", pod.Name)
			continue
		}

		stats, err := getPodNetstats(opts, &pod)
		if err != nil {
			logrus.Warnf("Could not get stats for pod %v: %v", pod.Name, err)
			continue
		}
		podStats = append(podStats, &metrics.PodStats{
			NetStats:  *stats,
			Name:      pod.Name,
			Namespace: pod.Namespace,
		})
	}

	return podStats, nil
}

func main() {
	opts := &ops{}
	parser := flags.NewParser(opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		// If the error was from the parser, then we can simply return
		// as Parse() prints the error already
		if _, ok := err.(*flags.Error); ok {
			os.Exit(1)
		}
		logrus.Fatalf("Error parsing flags: %v", err)
	}
	setupLogging(opts.LogLevel)

	client, err := kubelet.NewClient(opts.ClientConfig)
	if err != nil {
		logrus.Fatal(err)
	}

	srv := &http.Server{
		Addr: opts.BindAddr,
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK\n")
	})
	http.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK\n")
	})

	metricsLimiter := rate.NewLimiter(rate.Limit(opts.RateLimit), 5)
	http.HandleFunc("/metrics", func(rsp http.ResponseWriter, req *http.Request) {
		if metricsLimiter.Allow() == false {
			http.Error(rsp, http.StatusText(429), http.StatusTooManyRequests)
			return
		}

		stats, err := allPodStats(opts, client)
		if err != nil {
			logrus.Error(err)
			metrics.HTTPError(rsp, err)
			return
		}

		metrics.Handler(rsp, req, stats)
	})
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			logrus.Errorf("Error serving HTTP at %v: %v", opts.BindAddr, err)
		}
	}()

	stopCh := make(chan struct{})
	defer close(stopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm

	logrus.Info("Received SIGTERM or SIGINT. Shutting down.")
	srv.Shutdown(context.Background())
}
