// The sync_http_error_code_configmap_controller is responsible for:
//
//   1. Synchronize the configmaps created for custom error code pages between
//  admin created config in openshift-config namespace and openshift-ingress namespace
//   3. Publishing the CA to `openshift-config-managed`
package sync_http_error_code_configmap

import (
	"context"
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	logf "github.com/openshift/cluster-ingress-operator/pkg/log"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller"
	ingresscontroller "github.com/openshift/cluster-ingress-operator/pkg/operator/controller/ingress"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimecontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "sync_http_error_code_configmap_controller"
)

var log = logf.Logger.WithName(controllerName)

func New(mgr manager.Manager, operatorNamespace string, operandNamespace string, sourceConfigMapNamespace string) (runtimecontroller.Controller, error) {
	operatorCache := mgr.GetCache()
	reconciler := &reconciler{
		client:                   mgr.GetClient(),
		cache:                    operatorCache,
		recorder:                 mgr.GetEventRecorderFor(controllerName),
		operatorNamespace:        operatorNamespace,
		operandNamespace:         operandNamespace,
		sourceConfigMapNamespace: sourceConfigMapNamespace,
	}
	c, err := runtimecontroller.New(controllerName, mgr, runtimecontroller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &operatorv1.IngressController{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	// Index ingresscontrollers over the httpErrorCodePage name so that
	// configMapIsInUse and configMapToIngressController can look up
	// ingresscontrollers that reference the secret.
	if err := operatorCache.IndexField(context.Background(), &operatorv1.IngressController{}, "httpErrorCodePages", func(o client.Object) []string {
		configmapInOpenShiftConfig := controller.HttpErrorCodePageConfigMapName(o.(*operatorv1.IngressController), sourceConfigMapNamespace)
		configmapInOpenShiftIngress := controller.HttpErrorCodePageConfigMapName(o.(*operatorv1.IngressController), operandNamespace)
		return []string{configmapInOpenShiftConfig.Name, configmapInOpenShiftIngress.Name}
	}); err != nil {
		return nil, fmt.Errorf("failed to create index for ingresscontroller: %v", err)
	}

	configmapsInformerForOpenShiftConfigAndOpenShiftIngress, err := operatorCache.GetInformer(context.Background(), &corev1.ConfigMap{})
	if err != nil {
		return nil, fmt.Errorf("failed to create informer for configMap: %v", err)
	}
	if err := c.Watch(&source.Informer{Informer: configmapsInformerForOpenShiftConfigAndOpenShiftIngress}, handler.EnqueueRequestsFromMapFunc(reconciler.configmapToIngressController), predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return reconciler.configmapIsInUse(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return reconciler.configmapIsInUse(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return reconciler.configmapIsInUse(e.ObjectNew) },
		GenericFunc: func(e event.GenericEvent) bool { return reconciler.configmapIsInUse(e.Object) },
	}); err != nil {
		return nil, err
	}

	return c, nil
}

type reconciler struct {
	client                   client.Client
	recorder                 record.EventRecorder
	operatorNamespace        string
	cache                    cache.Cache
	operandNamespace         string
	sourceConfigMapNamespace string
}

// configmapToIngressController maps a configmap to a slice of reconcile requests,
// one request per ingresscontroller that references the configmap.
func (r *reconciler) configmapToIngressController(o client.Object) []reconcile.Request {
	requests := []reconcile.Request{}
	controllers, err := r.ingressControllersWithConfigMap(o.GetName())
	if err != nil {
		log.Error(err, "failed to list ingresscontrollers for configmap", "related", o.GetSelfLink())
		return requests
	}
	for _, ic := range controllers {
		log.Info("queueing ingresscontroller", "name", ic.Name, "related", o.GetSelfLink())
		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: ic.Namespace,
				Name:      ic.Name,
			},
		}
		requests = append(requests, request)
	}
	return requests
}

// ingressControllersWithConfigMap returns the ingresscontrollers that reference
// the given configmap.
func (r *reconciler) ingressControllersWithConfigMap(configmapName string) ([]operatorv1.IngressController, error) {
	controllers := &operatorv1.IngressControllerList{}
	if err := r.cache.List(context.Background(), controllers); err != nil {
		return nil, err
	}
	return controllers.Items, nil
}

// configmapIsInUse returns true if the given configmap is referenced by some
// ingresscontroller.
func (r *reconciler) configmapIsInUse(meta metav1.Object) bool {
	controllers, err := r.ingressControllersWithConfigMap(meta.GetName())
	if err != nil {
		log.Error(err, "failed to list ingresscontrollers for configmap", "related", meta.GetSelfLink())
		return false
	}
	return len(controllers) > 0
}

// hasConfigMap returns true if the effective  httpErrorCodePage configmap for the
// given ingresscontroller exists, false otherwise.
func (r *reconciler) hasConfigMap(meta metav1.Object, o runtime.Object) bool {
	ic := o.(*operatorv1.IngressController)
	configMapName := controller.HttpErrorCodePageConfigMapName(ic, r.operandNamespace)
	configMap := &corev1.ConfigMap{}
	if err := r.client.Get(context.Background(), configMapName, configMap); err != nil {
		if errors.IsNotFound(err) {
			return false
		}
		log.Error(err, "failed to look up configmap for ingresscontroller", "name", configMapName, "related", meta.GetSelfLink())
	}
	return true
}

// configMapChanged returns true if the name of config
// for the given ingresscontroller has changed, false
// otherwise.
// TODO check the data between old and new configmaps
func (r *reconciler) configMapChanged(old, new runtime.Object) bool {
	oldController := old.(*operatorv1.IngressController)
	newController := new.(*operatorv1.IngressController)
	oldConfigMap := controller.HttpErrorCodePageConfigMapName(oldController, r.sourceConfigMapNamespace)
	newConfigMap := controller.HttpErrorCodePageConfigMapName(newController, r.sourceConfigMapNamespace)
	oldStatus := oldController.Spec.HttpErrorCodePages.Name
	newStatus := newController.Spec.HttpErrorCodePages.Name
	return oldConfigMap != newConfigMap || oldStatus != newStatus
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	result := reconcile.Result{}
	errs := []error{}
	ingress := &operatorv1.IngressController{}
	if err := r.client.Get(context.TODO(), request.NamespacedName, ingress); err != nil {
		if errors.IsNotFound(err) {
			// The ingress could have been deleted and we're processing a stale queue
			// item, so ignore and skip.
			log.Info("ingresscontroller not found; reconciliation will be skipped", "request", request)
		} else {
			errs = append(errs, fmt.Errorf("failed to get ingresscontroller: %v", err))
		}
	} else if !ingresscontroller.IsStatusDomainSet(ingress) {
		log.Info("ingresscontroller domain not set; reconciliation will be skipped", "request", request)
	} else {
		deployment := &appsv1.Deployment{}
		err = r.client.Get(context.TODO(), controller.RouterDeploymentName(ingress), deployment)
		if err != nil {
			if errors.IsNotFound(err) {
				// All ingresses should have a deployment, so this one may not have been
				// created yet. Retry after a reasonable amount of time.
				log.Info("deployment not found; will retry default cert sync", "ingresscontroller", ingress.Name)
				result.RequeueAfter = 5 * time.Second
			} else {
				errs = append(errs, fmt.Errorf("failed to get deployment: %v", err))
			}
		} else {
			trueVar := true
			deploymentRef := metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
				UID:        deployment.UID,
				Controller: &trueVar,
			}
			controllers := &operatorv1.IngressControllerList{}
			if err := r.cache.List(context.TODO(), controllers, client.InNamespace(r.operatorNamespace)); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to list ingresscontrollers: %v", err)
			}
			for _, ingresscontroller := range controllers.Items {
				if _, _, err := r.ensureHttpErrorCodeConfigMap(&ingresscontroller, deploymentRef); err != nil {
					errs = append(errs, fmt.Errorf("failed to ensure default cert for %s: %v", ingress.Name, err))
				}
			}
		}
	}
	return result, utilerrors.NewAggregate(errs)
}

// CurrentHttpErrorCodeConfigMap returns the current configmap.  Returns a
// Boolean indicating whether the configmap existed, the configmap if it did
// exist, and an error value.
func (r *reconciler) currentHttpErrorCodeConfigMap(ic *operatorv1.IngressController, namespace string) (bool, *corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(context.TODO(), controller.HttpErrorCodePageConfigMapName(ic, namespace), cm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, cm, nil
}
