package sync_http_error_code_configmap

import (
	"context"
	"fmt"
	"reflect"

	operatorv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ensureHttpErrorCodeConfigMap ensures the http error code configmap exists for a given
// ingresscontroller and sync them between openshift-config and openshift-ingress.  Returns a Boolean
// indicating whether the configmap exists, the configmap if it does exist, and
// an error value.
func (r *reconciler) ensureHttpErrorCodeConfigMap(ic *operatorv1.IngressController, deploymentRef metav1.OwnerReference) (bool, *corev1.ConfigMap, error) {
	haveCMInOpenShiftConfig, currentConfigMapInOpenShiftConfig, err := r.currentHttpErrorCodeConfigMap(ic, "openshift-config")
	if err != nil {
		return false, nil, err
	}
	haveCMInOpenShiftIngress, currentConfigMapInOpenShiftIngress, err := r.currentHttpErrorCodeConfigMap(ic, "openshift-ingress")
	if err != nil {
		return false, nil, err
	}
	switch {
	case len(ic.Spec.HttpErrorCodePages.Name) != 0 && !haveCMInOpenShiftConfig && !haveCMInOpenShiftIngress:
		return false, nil, nil
	case len(ic.Spec.HttpErrorCodePages.Name) != 0 && !haveCMInOpenShiftConfig && haveCMInOpenShiftIngress:
		if err := r.client.Delete(context.TODO(), currentConfigMapInOpenShiftIngress); err != nil {
			if !errors.IsNotFound(err) {
				return true, currentConfigMapInOpenShiftIngress, fmt.Errorf("failed to delete configmap: %v", err)
			}
		} else {
			log.Info("deleted configmap", "configmap", currentConfigMapInOpenShiftIngress)
		}
		return false, nil, nil
	case len(ic.Spec.HttpErrorCodePages.Name) != 0 && haveCMInOpenShiftConfig && !haveCMInOpenShiftIngress:
		_, currentConfigMapInOpenShiftIngress, err = desiredHttpErrorCodeConfigMap(currentConfigMapInOpenShiftConfig, deploymentRef)
		if err := r.client.Create(context.TODO(), currentConfigMapInOpenShiftIngress); err != nil {
			return false, nil, fmt.Errorf("failed to create configmap: %v", err)
		}
		log.Info("created configmap", "configmap", currentConfigMapInOpenShiftIngress)
		return r.currentHttpErrorCodeConfigMap(ic, "openshift-ingress")
	case len(ic.Spec.HttpErrorCodePages.Name) != 0 && haveCMInOpenShiftConfig && haveCMInOpenShiftIngress:
		if updated, err := r.updateHttpErrorCodeConfigMap(currentConfigMapInOpenShiftIngress, currentConfigMapInOpenShiftConfig); err != nil {
			return true, currentConfigMapInOpenShiftIngress, fmt.Errorf("failed to update configmap: %v", err)
		} else if updated {
			log.Info("updated configmap in openshift-ingress namespace", "configmap", currentConfigMapInOpenShiftIngress)
			return r.currentHttpErrorCodeConfigMap(ic, "openshift-ingress")
		}
	}

	return true, currentConfigMapInOpenShiftIngress, nil
}

// desiredRsyslogConfigMap returns the desired  httpErrorCodePageconfigmap.  Returns a
// Boolean indicating whether a configmap is desired, as well as the configmap
// if one is desired.
func desiredHttpErrorCodeConfigMap(currentConfigMapInOpenShiftConfig *corev1.ConfigMap, deploymentRef metav1.OwnerReference) (bool, *corev1.ConfigMap, error) {
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      currentConfigMapInOpenShiftConfig.Name,
			Namespace: "openshift-ingress",
		},
		Data: currentConfigMapInOpenShiftConfig.Data,
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{deploymentRef})
	return true, &cm, nil
}

// updateHttpErrorCodeConfigMap updates a httpErrorCodePage configmap.  Returns a Boolean indicating
// whether the configmap was updated, and an error value.
func (r *reconciler) updateHttpErrorCodeConfigMap(current, desired *corev1.ConfigMap) (bool, error) {
	if httperrorcodeConfigmapsEqual(current, desired) {
		return false, nil
	}
	updated := current.DeepCopy()
	updated.Data = desired.Data
	if err := r.client.Update(context.TODO(), updated); err != nil {
		if errors.IsAlreadyExists(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// httperrorcodeConfigmapsEqual compares two httpErrorCodePage configmaps between openshift-config and openshift-ingress.  Returns true if the
// configmaps should be considered equal for the purpose of determining whether
// an update is necessary, false otherwise
func httperrorcodeConfigmapsEqual(a, b *corev1.ConfigMap) bool {
	if !reflect.DeepEqual(a.Data, b.Data) {
		return false
	}
	return true
}
