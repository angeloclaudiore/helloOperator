package jedykind

import (
	"context"
	"math/rand"
	"time"

	cachev1alpha1 "github.com/ValentinoUberti/hello-operator/pkg/apis/cache/v1alpha1"

	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_jedykind")

type PodDbType struct {
	PodName  string
	IsMaster bool
}

func IsMaster(pod PodDbType) bool {

	return pod.IsMaster
}

type LivePodList []PodDbType

// Add creates a new JedyKind Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))

}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileJedyKind{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("jedykind-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource JedyKind
	err = c.Watch(&source.Kind{Type: &cachev1alpha1.JedyKind{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner JedyKind
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cachev1alpha1.JedyKind{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileJedyKind{}

// ReconcileJedyKind reconciles a JedyKind object
type ReconcileJedyKind struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a JedyKind object and makes changes based on the state read
// and what is in the JedyKind.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.

func (r *ReconcileJedyKind) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling JedyKind")
	const charset = "abcdefghijklmnopqrstuvwxyz"

	// Fetch the JedyKind instance
	// Here we have all the pods of JedyKind kind
	instance := &cachev1alpha1.JedyKind{}

	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Start custom logic by ValeUbe
	// Get the spec: size requested by user from CR yaml file
	// ./deploy/crds/cache_v1alpha1_jedykind_cr.yaml
	size := instance.Spec.Size
	logSizeStr := fmt.Sprintf("Requested size from CR : %d", size)
	reqLogger.Info(logSizeStr)

	// List all pods owned by this JedyKind instance
	podList := &corev1.PodList{}
	lbs := map[string]string{
		"app": instance.Name,
	}
	labelSelector := labels.SelectorFromSet(lbs)

	listOps := &client.ListOptions{Namespace: instance.Namespace, LabelSelector: labelSelector}
	if err = r.client.List(context.TODO(), listOps, podList); err != nil {
		return reconcile.Result{}, err
	}

	var available []corev1.Pod
	for _, pod := range podList.Items {

		if pod.ObjectMeta.DeletionTimestamp != nil {
			continue
		}

		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			available = append(available, pod)
		}
	}

	numAvailable := int32(len(available))

	logNumAvaible := fmt.Sprintf("Avaible pods : %d", numAvailable)
	reqLogger.Info(logNumAvaible)

	availableNames := []string{}

	createMaster := true

	for _, pod := range available {

		availableNames = append(availableNames, pod.ObjectMeta.Name)
		logAvaiblePodName := fmt.Sprintf("Current pod name : %s", pod.ObjectMeta.Name)
		reqLogger.Info(logAvaiblePodName)

		logNumAvaible = fmt.Sprintf("Pod's ip : %v", pod.Status.PodIP)
		reqLogger.Info(logNumAvaible)

		isMaster := pod.GetLabels()["isMaster"]

		if isMaster == "true" {
			createMaster = false
		}

		logPodLabes := fmt.Sprintf("Is Master : %v", isMaster)
		reqLogger.Info(string(logPodLabes))

	}

	if numAvailable > instance.Spec.Size {
		reqLogger.Info("Scaling down pods", "Currently available", numAvailable, "Required replicas", instance.Spec.Size)
		diff := numAvailable - instance.Spec.Size
		dpods := available[:diff]
		for _, dpod := range dpods {
			err = r.client.Delete(context.TODO(), &dpod)
			if err != nil {
				reqLogger.Error(err, "Failed to delete pod", "pod.name", dpod.Name)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: true}, nil
	}

	// Pod already exists - don't requeue
	//reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)

	if numAvailable < instance.Spec.Size {
		reqLogger.Info("Scaling up pods", "Currently available", numAvailable, "Required size", instance.Spec.Size)
		// Define a new Pod object
		labelServiceSelector := StringWithCharset(5, charset)

		pod := newPodForCR(instance, labelServiceSelector, createMaster)
		// Set PodSet instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Create(context.TODO(), pod)
		if err != nil {
			reqLogger.Error(err, "Failed to create pod", "pod.name", pod.Name)
			return reconcile.Result{}, err
		}

		service := newServiceForPod(instance, labelServiceSelector)
		// Set PodSet instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		err = r.client.Create(context.TODO(), service)
		if err != nil {
			reqLogger.Error(err, "Failed to create service", "service.name", service.Name)
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	//End custom logic by ValeUbe

	return reconcile.Result{}, nil
}

func StringWithCharset(length int, charset string) string {
	var seededRand *rand.Rand = rand.New(
		rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *cachev1alpha1.JedyKind, labelServiceSelector string, master bool) *corev1.Pod {

	uniqueLabel := cr.Name + "-slave-" + labelServiceSelector

	isMaster := "false"
	if master {
		isMaster = "true"
		uniqueLabel = cr.Name + "-master-" + labelServiceSelector
	}

	labels := map[string]string{
		"app":              cr.Name,
		"internal_service": labelServiceSelector,
		"isMaster":         isMaster,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueLabel,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}

func newServiceForPod(cr *cachev1alpha1.JedyKind, labelServiceSelector string) *corev1.Service {

	uniqueLabel := cr.Name + "-service-" + labelServiceSelector
	labels := map[string]string{
		"app":              cr.Name,
		"internal_service": labelServiceSelector,
	}

	serviceTargetPort := intstr.FromInt(3306)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uniqueLabel,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       26379,
					TargetPort: serviceTargetPort,
					Protocol:   "TCP",
				},
			},
		},
	}

}
