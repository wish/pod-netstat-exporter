package kubelet

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClientConfig holds the config options for connecting to the kubelet API
type ClientConfig struct {
	NodeName string `long:"node-name" env:"NODE_NAME" description:"Current node name" required:"true"`
}

// NewClient returns a new KubeletClient based on the given config
func NewClient(c ClientConfig) (*Client, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{Clientset: clientset, c: c}, nil
}

// Client is an HTTP client for kubelet that implements the Kubelet interface
type Client struct {
	*kubernetes.Clientset
	c ClientConfig
}

// GetPodList returns the list of pods the kubelet is managing
func (k *Client) GetPodList() (*corev1.PodList, error) {
	return k.Clientset.CoreV1().Pods("").List(metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + k.c.NodeName,
	})
}
