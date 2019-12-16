package kubelet

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"

	k8sApi "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

// ClientConfig holds the config options for connecting to the kubelet API
type ClientConfig struct {
	APIEndpoint        string `long:"kubelet-api" env:"KUBELET_API" description:"kubelet API endpoint" default:"http://localhost:10250/pods"`
	InsecureSkipVerify bool   `long:"kubelet-api-insecure-skip-verify" env:"KUBELET_API_INSECURE_SKIP_VERIFY" description:"skip verification of TLS certificate from kubelet API"`
}

// NewClient returns a new KubeletClient based on the given config
func NewClient(c ClientConfig) (*Client, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		if err == rest.ErrNotInCluster {
			if c.InsecureSkipVerify {
				tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
				return &Client{c: c, client: &http.Client{Transport: tr}}, nil
			}

			return &Client{c: c, client: http.DefaultClient}, nil
		}
		return nil, err
	}
	if c.InsecureSkipVerify {
		config.TLSClientConfig.Insecure = true
		config.TLSClientConfig.CAData = nil
		config.TLSClientConfig.CAFile = ""
	}
	transport, err := rest.TransportFor(config)
	if err != nil {
		return nil, err
	}

	return &Client{c: c, client: &http.Client{Transport: transport}}, nil
}

// Client is an HTTP client for kubelet that implements the Kubelet interface
type Client struct {
	c      ClientConfig
	client *http.Client
}

// GetPodList returns the list of pods the kubelet is managing
func (k *Client) GetPodList() (*k8sApi.PodList, error) {
	// k8s testing
	req, err := http.NewRequest("GET", k.c.APIEndpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var podList k8sApi.PodList
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &podList); err != nil {
		return nil, err
	}
	return &podList, nil
}
