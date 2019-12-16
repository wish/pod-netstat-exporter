# pod-netstat-exporter [![GoDoc](https://godoc.org/github.com/wish/pod-netstat-exporter?status.svg)](https://godoc.org/github.com/wish/pod-netstat-exporter) [![Go Report Card](https://goreportcard.com/badge/github.com/wish/pod-netstat-exporter)](https://goreportcard.com/report/github.com/wish/pod-netstat-exporter)  [![Docker Repository on Quay](https://quay.io/repository/wish/pod-netstat-exporter/status "Docker Repository on Quay")](https://quay.io/repository/wish/pod-netstat-exporter)

Get detailed, per-pod network metrics for export to prometheus. Right now it assumes that:

- You are using kubernetes, and the local kubelet API is accessible
- You are using docker

## Limitations

For the most part, a k8s pod maps 1-to-1 to a linux network namespace. This however is not
the case if the pod is using `hostNetwork: true`. In that case, the pod uses the default
network namespace of the host, and there are no per-pod metrics to collect. Rather than
report something misleading, `pod-netstat-exporter` just doesn't return any metrics for those
pods.
