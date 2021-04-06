package sync_http_error_code_configmap

import (
	"context"
	"fmt"
	"reflect"

	operatorcontroller "github.com/openshift/cluster-ingress-operator/pkg/operator/controller"

	operatorv1 "github.com/openshift/api/operator/v1"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ensureHttpErrorCodeConfigMap ensures the http error code configmap exists for
// a given ingresscontroller and syncs them between openshift-config and
// openshift-ingress.  Returns a Boolean indicating whether the configmap
// exists, the configmap if it does exist, and an error value.
func (r *reconciler) ensureHttpErrorCodeConfigMap(ic *operatorv1.IngressController, deploymentRef metav1.OwnerReference) (bool, *corev1.ConfigMap, error) {
	sourceName := types.NamespacedName{
		Namespace: operatorcontroller.GlobalUserSpecifiedConfigNamespace,
		Name:      ic.Spec.HttpErrorCodePages.Name,
	}
	haveSource, source, err := r.currentHttpErrorCodeConfigMap(sourceName)
	if err != nil {
		return false, nil, err
	}
	name := operatorcontroller.HttpErrorCodePageConfigMapName(ic)
	have, current, err := r.currentHttpErrorCodeConfigMap(name)
	if err != nil {
		return false, nil, err
	}
	want, desired, err := desiredHttpErrorCodeConfigMap(haveSource, source, name, deploymentRef)
	if err != nil {
		return have, current, err
	}
	switch {
	case !want && !have:
		return false, nil, nil
	case !want && have:
		if err := r.client.Delete(context.TODO(), current); err != nil {
			if !errors.IsNotFound(err) {
				return true, current, fmt.Errorf("failed to delete configmap: %w", err)
			}
		} else {
			log.Info("deleted configmap", "namespace", current.Namespace, "name", current.Name)
		}
		return false, nil, nil
	case want && !have:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create configmap %s/%s: %v", desired.Namespace, desired.Name, err)
		}
		log.Info("created configmap", "namespace", desired.Namespace, "name", desired.Name)
		return r.currentHttpErrorCodeConfigMap(name)
	case want && have:
		if updated, err := r.updateHttpErrorCodeConfigMap(current, desired); err != nil {
			return true, current, fmt.Errorf("failed to update configmap %s/%s: %v", current.Namespace, current.Name, err)
		} else if updated {
			return r.currentHttpErrorCodeConfigMap(name)
		}
	}

	return have, current, nil
}

// desiredHttpErrorCodeConfigMap returns the desired error-page configmap.
// Returns a Boolean indicating whether a configmap is desired, as well as the
// configmap if one is desired.
func desiredHttpErrorCodeConfigMap(haveSource bool, sourceConfigmap *corev1.ConfigMap, name types.NamespacedName, deploymentRef metav1.OwnerReference) (bool, *corev1.ConfigMap, error) {
	if !haveSource {
		return false, nil, nil
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Data: sourceConfigmap.Data,
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{deploymentRef})
	return true, &cm, nil
}

// currentHttpErrorCodeConfigMap returns the current configmap.  Returns a
// Boolean indicating whether the configmap existed, the configmap if it did
// exist, and an error value.
func (r *reconciler) currentHttpErrorCodeConfigMap(name types.NamespacedName) (bool, *corev1.ConfigMap, error) {
	if len(name.Name) == 0 {
		return false, nil, nil
	}
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(context.TODO(), name, cm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, cm, nil
}

// updateHttpErrorCodeConfigMap updates a configmap.  Returns a Boolean
// indicating whether the configmap was updated, and an error value.
func (r *reconciler) updateHttpErrorCodeConfigMap(current, desired *corev1.ConfigMap) (bool, error) {
	if configmapsEqual(current, desired) {
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
	log.Info("updated configmap", "namespace", updated.Namespace, "name", updated.Name)
	return true, nil
}

// configmapsEqual compares two httpErrorCodePage configmaps.  Returns true if
// the configmaps should be considered equal for the purpose of determining
// whether an update is necessary, false otherwise.
func configmapsEqual(a, b *corev1.ConfigMap) bool {
	if !reflect.DeepEqual(a.Data, b.Data) {
		return false
	}
	return true
}
