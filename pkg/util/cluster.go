package util

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/clay-wangzhi/kube-on-kube/api"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var ServiceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// GetCurrentNS fetch namespace the current pod running in. reference to client-go (config *inClusterClientConfig) Namespace() (string, bool, error).
func GetCurrentNS() (string, error) {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns, nil
	}

	if data, err := os.ReadFile(ServiceAccountNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns, nil
		}
	}
	return "", fmt.Errorf("can not get namespace where pods running in")
}

func GetCurrentNSOrDefault() string {
	ns, err := GetCurrentNS()
	if err != nil {
		return "default"
	}
	return ns
}

func UpdateOwnReference(client kubernetes.Interface, configMapList []*api.ConfigMapRef, secretList []*api.SecretRef, belongToReference metav1.OwnerReference) error {
	for _, ref := range configMapList {
		if ref.IsEmpty() {
			continue
		}
		cm, err := client.CoreV1().ConfigMaps(ref.NameSpace).Get(context.Background(), ref.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if len(cm.OwnerReferences) != 0 {
			continue
		}
		// cm belongs to `Cluster`
		cm.OwnerReferences = append(cm.OwnerReferences, belongToReference)
		if _, err := client.CoreV1().ConfigMaps(ref.NameSpace).Update(context.Background(), cm, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	for _, ref := range secretList {
		if ref.IsEmpty() {
			continue
		}
		secret, err := client.CoreV1().Secrets(ref.NameSpace).Get(context.Background(), ref.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) { // ignore
				continue
			}
			return err // not ignore
		}
		if len(secret.OwnerReferences) != 0 {
			continue // do nothing
		}
		secret.OwnerReferences = append(secret.OwnerReferences, belongToReference)
		if _, err := client.CoreV1().Secrets(ref.NameSpace).Update(context.Background(), secret, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}
