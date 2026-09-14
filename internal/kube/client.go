package kube

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NewClientset builds a typed clientset from a REST config.
func NewClientset(cfg *rest.Config) (client kubernetes.Interface, err error) {
	client, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		err = fmt.Errorf("error-building-clientset :%w", err)
		return client, err
	}

	return client, err
}
