package main

import (
	"context"
	"fmt"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	autoscalinglinformersv2 "k8s.io/client-go/informers/autoscaling/v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	autoscalinglistersv2 "k8s.io/client-go/listers/autoscaling/v2"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	customhpav1alpha1 "custom-hpa/pkg/apis/customhpa/v1alpha1"
	clientset "custom-hpa/pkg/generated/clientset/versioned"
	customhpascheme "custom-hpa/pkg/generated/clientset/versioned/scheme"
	informers "custom-hpa/pkg/generated/informers/externalversions/customhpa/v1alpha1"

	listers "custom-hpa/pkg/generated/listers/customhpa/v1alpha1"
)

const controllerAgentName = "custom-hpa-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a CustomHPA is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a CustomHPA fails
	// to sync due to a HolizontalPodAutoscaler of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a HolizontalPodAutoscaler already existing
	MessageResourceExists = "Resource %q already exists and is not managed by CustomHPA"
	// MessageResourceSynced is the message used for an Event fired when a CustomHPA
	// is synced successfully
	MessageResourceSynced = "CustomHPA synced successfully"
)

// Controller is the controller implementation for CustomHPA resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// customhpaclientset is a clientset for our own API group
	customhpaclientset            clientset.Interface
	holizontalPodAutoscalerLister autoscalinglistersv2.HorizontalPodAutoscalerLister
	holizontalPodAutoscalerSynced cache.InformerSynced
	customhpasLister              listers.CustomHPALister
	customhpasSynced              cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new customhpa controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	customhpaclientset clientset.Interface,
	holizontalPodAutoscalerInformer autoscalinglinformersv2.HorizontalPodAutoscalerInformer,

	customhpaInformer informers.CustomHPAInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add customhpa-controller types to the default Kubernetes Scheme so Events can be
	// logged for customhpa-controller types.
	utilruntime.Must(customhpascheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:                 kubeclientset,
		customhpaclientset:            customhpaclientset,
		holizontalPodAutoscalerLister: holizontalPodAutoscalerInformer.Lister(),
		holizontalPodAutoscalerSynced: holizontalPodAutoscalerInformer.Informer().HasSynced,
		customhpasLister:              customhpaInformer.Lister(),
		customhpasSynced:              customhpaInformer.Informer().HasSynced,
		workqueue:                     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "CustomHPAs"),
		recorder:                      recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when CustomHPA resources change
	customhpaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueCustomHPA,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueCustomHPA(new)
		},
	})
	// Set up an event handler for when HolizontalPodAutoscaler resources change. This
	// handler will lookup the owner of the given HolizontalPodAutoscaler, and if it is
	// owned by a CustomHPA resource then the handler will enqueue that CustomHPA resource for
	// processing. This way, we don't need to implement custom logic for
	// handling HolizontalPodAutoscaler resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	holizontalPodAutoscalerInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*autoscalingv2.HorizontalPodAutoscaler)
			oldDepl := old.(*autoscalingv2.HorizontalPodAutoscaler)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known HolizontalPodAutoscalers.
				// Two different versions of the same HolizontalPodAutoscaler will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting CustomHPA controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.holizontalPodAutoscalerSynced, c.customhpasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process CustomHPA resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// CustomHPA resource to be synced.
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the CustomHPA resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the CustomHPA resource with this namespace/name
	customhpa, err := c.customhpasLister.CustomHPAs(namespace).Get(name)
	if err != nil {
		// The CustomHPA resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("customhpa '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	horizontalPodAutoscalerName := customhpa.Spec.HorizontalPodAutoscalerName
	if horizontalPodAutoscalerName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: holizontalPodAutoscaler name must be specified", key))
		return nil
	}

	// Get the horizontalPodAutoscaler with the name specified in CustomHPA.spec
	horizontalPodAutoscaler, err := c.holizontalPodAutoscalerLister.HorizontalPodAutoscalers(customhpa.Namespace).Get(horizontalPodAutoscalerName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		horizontalPodAutoscaler, err = c.kubeclientset.AutoscalingV2().HorizontalPodAutoscalers(customhpa.Namespace).Create(context.TODO(), newHorizontalPodAutoscaler(customhpa), metav1.CreateOptions{})
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the HolizontalPodAutoscaler is not controlled by this CustomHPA resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(horizontalPodAutoscaler, customhpa) {
		msg := fmt.Sprintf(MessageResourceExists, horizontalPodAutoscaler.Name)
		c.recorder.Event(customhpa, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// If this number of the replicas on the CustomHPA resource is specified, and the
	// number does not equal the current desired replicas on the HolizontalPodAutoscaler, we
	// should update the HolizontalPodAutoscaler resource.
	if customhpa.Spec.MinReplicas != nil && *customhpa.Spec.MinReplicas != *horizontalPodAutoscaler.Spec.MinReplicas {
		logger.V(4).Info("Update horizontalPodAutoscaler resource", "currentReplicas", *customhpa.Spec.MinReplicas, "desiredReplicas", *horizontalPodAutoscaler.Spec.MinReplicas)
		horizontalPodAutoscaler, err = c.kubeclientset.AutoscalingV2().HorizontalPodAutoscalers(customhpa.Namespace).Update(context.TODO(), newHorizontalPodAutoscaler(customhpa), metav1.UpdateOptions{})
	}

	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// Finally, we update the status block of the CustomHPA resource to reflect the
	// current state of the world
	err = c.updateCustomHPAStatus(customhpa)
	if err != nil {
		return err
	}

	c.recorder.Event(customhpa, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateCustomHPAStatus(customhpa *customhpav1alpha1.CustomHPA) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	customhpaCopy := customhpa.DeepCopy()
	// customhpaCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the CustomHPA resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.customhpaclientset.CustomhpaV1alpha1().CustomHPAs(customhpa.Namespace).UpdateStatus(context.TODO(), customhpaCopy, metav1.UpdateOptions{})
	return err
}

// enqueueCustomHPA takes a CustomHPA resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than CustomHPA.
func (c *Controller) enqueueCustomHPA(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the CustomHPA resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that CustomHPA resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a CustomHPA, we should not do anything more
		// with it.
		if ownerRef.Kind != "CustomHPA" {
			return
		}

		customhpa, err := c.customhpasLister.CustomHPAs(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "customhpa", ownerRef.Name)
			return
		}

		c.enqueueCustomHPA(customhpa)
		return
	}
}

// newHorizontalPodAutoscaler creates a new HorizontalPodAutoscaler for a CustomHPA resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the CustomHPA resource that 'owns' it.
func newHorizontalPodAutoscaler(customhpa *customhpav1alpha1.CustomHPA) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      customhpa.Spec.HorizontalPodAutoscalerName,
			Namespace: customhpa.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(customhpa, customhpav1alpha1.SchemeGroupVersion.WithKind("CustomHPA")),
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas:    customhpa.Spec.MinReplicas,
			MaxReplicas:    customhpa.Spec.MaxReplicas,
			ScaleTargetRef: customhpa.Spec.ScaleTargetRef,
			Metrics:        customhpa.Spec.Metrics,
			Behavior:       customhpa.Spec.Behavior.DeepCopy(),
		},
	}

}
